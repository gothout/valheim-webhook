package integracao

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"valheim-webhook/internal/infra/notificador/discord"
	"valheim-webhook/internal/pkg/config"
)

// Dependencias é o que o boot liga neste subdomínio.
type Dependencias struct {
	// Base é a configuração do `configs.json`: a SEMENTE da primeira execução
	// e a fonte dos parâmetros que o painel não edita (prazo, tamanho da fila).
	Base config.Discord
	// TiposValidos é o vocabulário de eventos aceito na lista de notificação.
	// Vem de fora porque o subdomínio de eventos é um IRMÃO — e irmão não se
	// importa; o `cmd/bootstrap`, que enxerga os dois, faz a ligação.
	TiposValidos []string
	// TiposPadrao é o que vale quando a lista está vazia.
	TiposPadrao []string
}

// Service concentra as regras do subdomínio.
type Service interface {
	// Ler devolve a configuração vigente (banco, ou a semente do arquivo).
	Ler(ctx context.Context) (*Integracao, error)
	// Salvar valida, grava e RECONFIGURA o publicador na hora.
	Salvar(ctx context.Context, req SalvarIntegracaoRequestDto, autor uuid.UUID) (*Integracao, error)
	// Testar publica uma mensagem de teste no destino configurado.
	Testar(ctx context.Context, autor string) error
	// Aplicar leva a configuração vigente ao publicador (chamado no boot).
	Aplicar(ctx context.Context) error
	// EventosPadrao expõe a lista padrão (o painel a exibe como sugestão).
	EventosPadrao() []string
	// Notificaveis devolve os tipos que hoje geram mensagem.
	Notificaveis(ctx context.Context) ([]string, error)
}

type serviceImpl struct {
	repo Repository
	deps Dependencias
}

// NewService monta o serviço.
func NewService(repo Repository, deps Dependencias) Service {
	return &serviceImpl{repo: repo, deps: deps}
}

func (s *serviceImpl) Ler(ctx context.Context) (*Integracao, error) {
	i, err := s.repo.Buscar(ctx, ServicoDiscord)
	switch {
	case err == nil:
		return i, nil
	case errors.Is(err, ErrNotFound):
		// Nunca foi salva pelo painel: o que vale é o arquivo. A semente NÃO é
		// gravada aqui — gravá-la copiaria o segredo do arquivo para o banco
		// sem ninguém ter pedido, e passaria a haver duas cópias dele.
		return s.semente(), nil
	default:
		return nil, obs.Observe(ctx, err)
	}
}

func (s *serviceImpl) Salvar(ctx context.Context, req SalvarIntegracaoRequestDto, autor uuid.UUID) (*Integracao, error) {
	atual, err := s.Ler(ctx)
	if err != nil {
		return nil, err
	}

	modo := strings.ToLower(strings.TrimSpace(req.Modo))
	if modo != ModoBot && modo != ModoWebhook {
		return nil, obs.Observe(ctx, ErrModoInvalido)
	}

	nova := &Integracao{
		UUID:          atual.UUID,
		Servico:       ServicoDiscord,
		Habilitado:    req.Habilitado,
		Modo:          modo,
		WebhookURL:    strings.TrimSpace(aplicarEm(atual.WebhookURL, req.WebhookURL)),
		BotToken:      strings.TrimSpace(aplicarEm(atual.BotToken, req.BotToken)),
		CanalID:       strings.TrimSpace(aplicarEm(atual.CanalID, req.CanalID)),
		Username:      strings.TrimSpace(req.Username),
		AtualizadoPor: ponteiroDe(autor),
		AtualizadoEm:  time.Now().UTC(),
	}
	if nova.UUID == uuid.Nil {
		nova.UUID = uuid.New()
	}

	eventos, err := s.validarEventos(req.Eventos)
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}
	nova.Eventos = strings.Join(eventos, ",")

	if nova.Modo == ModoWebhook && nova.WebhookURL != "" {
		if err := validarWebhook(nova.WebhookURL); err != nil {
			return nil, obs.Observe(ctx, err)
		}
	}
	// Ligar sem destino é o erro que produziria uma tela dizendo "ativa" e um
	// canal em silêncio — recusar é mais honesto do que aceitar e não avisar.
	if nova.Habilitado && !nova.TemDestino() {
		return nil, obs.Observe(ctx, ErrSemDestino)
	}

	if err := s.repo.Salvar(ctx, nova); err != nil {
		return nil, obs.Observe(ctx, err)
	}

	s.reconfigurar(*nova)
	return nova, nil
}

func (s *serviceImpl) Testar(ctx context.Context, autor string) error {
	vigente, err := s.Ler(ctx)
	if err != nil {
		return err
	}
	if !vigente.Ativa() {
		return obs.Observe(ctx, ErrNaoConfigurada)
	}

	cliente := discord.Use()
	if !cliente.Disponivel() {
		return obs.Observe(ctx, ErrNaoConfigurada)
	}

	cliente.Enviar(discord.Mensagem{
		Embed: &discord.Embed{
			Titulo:    "Integração conectada",
			Descricao: "Mensagem de teste enviada pelo painel do valheim-webhook.",
			Cor:       0x4CAF50,
			Rodape:    "solicitado por " + autor,
			Quando:    time.Now().UTC(),
		},
	})
	return nil
}

func (s *serviceImpl) Aplicar(ctx context.Context) error {
	vigente, err := s.Ler(ctx)
	if err != nil {
		return err
	}
	s.reconfigurar(*vigente)
	return nil
}

func (s *serviceImpl) EventosPadrao() []string { return s.deps.TiposPadrao }

func (s *serviceImpl) Notificaveis(ctx context.Context) ([]string, error) {
	vigente, err := s.Ler(ctx)
	if err != nil {
		return nil, err
	}
	lista := vigente.ListaDeEventos()
	if len(lista) == 0 {
		return s.deps.TiposPadrao, nil
	}
	return lista, nil
}

// reconfigurar leva a configuração ao publicador do processo.
func (s *serviceImpl) reconfigurar(i Integracao) {
	discord.Reconfigurar(s.ParaConfig(i))
	slog.Info("[INTEGRACAO] publicador do Discord reconfigurado",
		"habilitado", i.Habilitado, "modo", i.Modo, "ativa", i.Ativa())
}

// ParaConfig traduz a entidade na configuração que o publicador entende.
//
// O que o painel NÃO edita (prazo da chamada, tamanho da fila) vem do arquivo:
// são parâmetros de operação, não de integração, e expô-los na tela só criaria
// campos que ninguém sabe responder.
func (s *serviceImpl) ParaConfig(i Integracao) config.Discord {
	cfg := s.deps.Base
	cfg.Enabled = i.Habilitado
	cfg.Username = i.Username
	cfg.Eventos = i.ListaDeEventos()

	if i.Modo == ModoBot {
		cfg.BotToken, cfg.ChannelID = i.BotToken, i.CanalID
		// O webhook é zerado de propósito: com os dois preenchidos o publicador
		// escolheria o bot de qualquer forma, mas deixar o outro destino vivo na
		// configuração é o tipo de coisa que engana na hora de depurar.
		cfg.WebhookURL = ""
	} else {
		cfg.WebhookURL = i.WebhookURL
		cfg.BotToken, cfg.ChannelID = "", ""
	}
	return cfg
}

// semente monta a configuração inicial a partir do `configs.json`.
func (s *serviceImpl) semente() *Integracao {
	base := s.deps.Base
	modo := ModoWebhook
	if base.ModoBot() {
		modo = ModoBot
	}
	return &Integracao{
		Servico:    ServicoDiscord,
		Habilitado: base.Enabled,
		Modo:       modo,
		WebhookURL: strings.TrimSpace(base.WebhookURL),
		BotToken:   strings.TrimSpace(base.BotToken),
		CanalID:    strings.TrimSpace(base.ChannelID),
		Username:   base.Username,
		Eventos:    strings.Join(base.Eventos, ","),
	}
}

// validarEventos confere a lista contra o vocabulário aceito.
func (s *serviceImpl) validarEventos(pedidos []string) ([]string, error) {
	validos := make(map[string]struct{}, len(s.deps.TiposValidos))
	for _, tipo := range s.deps.TiposValidos {
		validos[tipo] = struct{}{}
	}

	var lista []string
	for _, pedido := range pedidos {
		limpo := strings.ToLower(strings.TrimSpace(pedido))
		if limpo == "" {
			continue
		}
		if _, ok := validos[limpo]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrEventoInvalido, limpo)
		}
		lista = append(lista, limpo)
	}
	return lista, nil
}

// validarWebhook confere que a URL é um webhook do Discord.
//
// A checagem existe para pegar o erro comum (colar a URL do CANAL em vez da do
// webhook), que produziria um 404 silencioso na fila de envio.
func validarWebhook(bruta string) error {
	endereco, err := url.Parse(bruta)
	if err != nil || endereco.Scheme != "https" || endereco.Host == "" {
		return ErrURLInvalida
	}
	if !strings.Contains(endereco.Path, "/api/webhooks/") {
		return ErrURLInvalida
	}
	return nil
}

// ponteiroDe devolve nil para o UUID zerado (autor desconhecido).
func ponteiroDe(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
