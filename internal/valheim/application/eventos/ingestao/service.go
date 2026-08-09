// Package ingestao é o caso de uso que recebe uma linha de log do servidor de
// Valheim e a transforma em tudo o que ela deve produzir: um registro no
// diário, uma atualização do estado do personagem, uma mensagem no Discord e
// uma linha no painel de quem está com a tela aberta.
//
// Ele mora na camada de APLICAÇÃO porque costura DOIS subdomínios de domínio
// (`eventos/evento` e `mundo/jogador`) mais a política de notificação
// (`configuracao/integracao`) — e nenhum dos três pode conhecer os outros.
//
// A ordem das quatro coisas não é arbitrária:
//
//  1. GRAVAR o evento. É a única etapa cujo fracasso vira erro para quem
//     chamou: se o diário não recebeu, a linha se perdeu de verdade;
//  2. ATUALIZAR o personagem. Falha aqui é registrada e seguida — o diário já
//     tem o fato, e o saldo se corrige no próximo evento;
//  3. PUBLICAR no painel. Nunca falha (o barramento descarta em silêncio);
//  4. NOTIFICAR o Discord. Só enfileira.
//
// O que amarra tudo: o hook do container tem um `curl` com prazo e o servidor
// de jogo não espera por ninguém. Nada aqui pode bloquear.
package ingestao

import (
	"context"
	"errors"
	"strings"
	"time"

	"valheim-webhook/internal/infra/notificador/discord"
	"valheim-webhook/internal/pkg/tempo_real"
)

// MaxLinhasPorLote é o teto de linhas de uma chamada.
//
// Existe porque `text/plain` aceita um corpo com muitas linhas, e uma chamada
// que vira quinhentas gravações seguras seguraria a conexão do hook por tempo
// demais. Quem tem mais do que isso para mandar, manda em duas.
const MaxLinhasPorLote = 100

// Resultado é o que uma linha produziu.
type Resultado struct {
	Registro   Registro
	Presenca   Presenca
	Notificado bool
}

// Service concentra as regras do caso de uso.
type Service interface {
	// Registrar processa UMA linha de log.
	Registrar(ctx context.Context, linha string) (*Resultado, error)
	// RegistrarLote processa várias linhas, na ordem em que chegaram.
	RegistrarLote(ctx context.Context, linhas []string) ([]Resultado, error)
	// Servidor é o nome com que os eventos são carimbados.
	Servidor() string
	// Hub é o barramento do painel, entregue ao controller para ele registrar a
	// rota SSE. Nulo quando o tempo real está desligado.
	Hub() *tempo_real.Hub
}

type serviceImpl struct {
	deps  Dependencias
	agora func() time.Time
}

// NewService monta o serviço sobre as dependências ligadas pelo boot.
func NewService(deps Dependencias) Service {
	return &serviceImpl{deps: deps, agora: func() time.Time { return time.Now().UTC() }}
}

func (s *serviceImpl) Servidor() string { return s.deps.Servidor }

func (s *serviceImpl) Registrar(ctx context.Context, linha string) (*Resultado, error) {
	if strings.TrimSpace(linha) == "" {
		return nil, obs.Observe(ctx, ErrCorpoVazio)
	}

	registro, err := s.deps.Eventos.Registrar(ctx, s.deps.Servidor, linha)
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}

	presenca := s.aplicarPresenca(ctx, registro)
	s.publicar(registro, presenca)
	notificado := s.notificar(ctx, registro, presenca)

	return &Resultado{Registro: registro, Presenca: presenca, Notificado: notificado}, nil
}

func (s *serviceImpl) RegistrarLote(ctx context.Context, linhas []string) ([]Resultado, error) {
	if len(linhas) == 0 {
		return nil, obs.Observe(ctx, ErrCorpoVazio)
	}
	if len(linhas) > MaxLinhasPorLote {
		return nil, obs.Observe(ctx, ErrLoteGrande)
	}

	resultados := make([]Resultado, 0, len(linhas))
	for _, linha := range linhas {
		if strings.TrimSpace(linha) == "" {
			continue // linha em branco no meio do lote não é erro do lote
		}
		resultado, err := s.Registrar(ctx, linha)
		if err != nil {
			return nil, err
		}
		resultados = append(resultados, *resultado)
	}
	if len(resultados) == 0 {
		return nil, obs.Observe(ctx, ErrCorpoVazio)
	}
	return resultados, nil
}

// aplicarPresenca leva o evento ao estado do personagem.
//
// Nenhum erro daqui interrompe a ingestão: o diário JÁ registrou o fato, e o
// saldo é derivado. Perder a atualização de um saldo é um número errado na
// tela até o próximo evento; abortar por causa dela seria perder o fato.
func (s *serviceImpl) aplicarPresenca(ctx context.Context, r Registro) Presenca {
	quando := r.OcorridoEm
	if quando.IsZero() {
		quando = s.agora()
	}

	var (
		presenca Presenca
		err      error
	)

	switch r.Tipo {
	case tipoEntrou:
		presenca, err = s.deps.Jogadores.Entrou(ctx, r.Servidor, r.Jogador, r.ZDOID, quando)

	case tipoSaiu:
		// A linha de saída traz o ZDOID; o nome, quando existe, veio de uma
		// consulta que o subdomínio de eventos já fez. Tenta pelo objeto (que é
		// o que identifica a SESSÃO) e cai para o nome.
		presenca, err = s.deps.Jogadores.SaiuPorZDOID(ctx, r.Servidor, r.ZDOID, quando)
		if err != nil && r.Jogador != "" {
			presenca, err = s.deps.Jogadores.SaiuPorNome(ctx, r.Servidor, r.Jogador, quando)
		}

	case tipoMorreu:
		err = s.deps.Jogadores.Morreu(ctx, r.Servidor, r.Jogador, quando)
		presenca = Presenca{Nome: r.Jogador, Online: true}

	case tipoMensagem:
		if r.Jogador == "" {
			return Presenca{}
		}
		err = s.deps.Jogadores.Atividade(ctx, r.Servidor, r.Jogador, quando)
		presenca = Presenca{Nome: r.Jogador, Online: true}

	case tipoServidorPronto:
		// O mundo subiu do zero: ninguém está mais lá dentro, e as sessões que
		// ficaram abertas no banco (porque o servidor caiu sem avisar) precisam
		// ser fechadas — senão o painel mostra gente online para sempre.
		_, err = s.deps.Jogadores.ServidorReiniciou(ctx, r.Servidor, quando)

	default:
		return Presenca{Nome: r.Jogador}
	}

	// Presença desconhecida NÃO é falha e não polui o log: o servidor limpa
	// objetos de sessões antigas ao reiniciar, e nenhum personagem online é
	// dono deles. Acontece em toda subida do mundo.
	if err != nil && !errors.Is(err, ErrPresencaDesconhecida) {
		_ = obs.Observe(ctx, err)
	}
	if presenca.Nome == "" {
		presenca.Nome = r.Jogador
	}
	return presenca
}

// publicar manda o evento para o painel. Hub nulo desliga o tempo real.
func (s *serviceImpl) publicar(r Registro, p Presenca) {
	if s.deps.Hub == nil {
		return
	}
	s.deps.Hub.Publicar(EventoSSE, paraAoVivo(r, p))
}

// notificar enfileira a mensagem do Discord quando o tipo está na política.
//
// Devolve se a mensagem foi ENFILEIRADA — não se ela chegou. Quem garante a
// entrega é o publicador, de forma assíncrona, e é no log dele que uma recusa
// do Discord aparece.
func (s *serviceImpl) notificar(ctx context.Context, r Registro, p Presenca) bool {
	cliente := discord.Use()
	if !cliente.Disponivel() {
		return false
	}
	if !s.notificavel(ctx, r, p) {
		return false
	}

	mensagem, vale := paraMensagem(r, p)
	if !vale {
		return false
	}
	cliente.Enviar(mensagem)
	return true
}

// notificavel decide se este evento vira mensagem.
//
// Duas perguntas, nessa ordem: o TIPO está na política configurada no painel? E,
// para entrada e saída, houve MUDANÇA de estado? A segunda é o que impede o
// canal de receber "Fulano entrou" a cada vez que a pessoa renasce numa cama.
func (s *serviceImpl) notificavel(ctx context.Context, r Registro, p Presenca) bool {
	if (r.Tipo == tipoEntrou || r.Tipo == tipoSaiu) && !p.Mudou {
		return false
	}

	tipos, err := s.deps.Politica.Notificaveis(ctx)
	if err != nil {
		// Sem conseguir ler a política, o silêncio é a escolha segura: mandar
		// tudo poderia despejar centenas de mensagens num canal.
		_ = obs.Observe(ctx, errors.Join(ErrRegistro, err))
		return false
	}
	for _, tipo := range tipos {
		if tipo == r.Tipo {
			return true
		}
	}
	return false
}

// Hub expõe o barramento para o controller registrar a rota SSE.
func (s *serviceImpl) Hub() *tempo_real.Hub { return s.deps.Hub }
