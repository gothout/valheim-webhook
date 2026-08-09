package ingestao

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"valheim-webhook/internal/pkg/tempo_real"
)

// Os testes deste arquivo rodam SEM banco, SEM Discord e SEM HTTP — é para isso
// que os vizinhos entram por interface (`contratos.go`) em vez de import.

// ---------- dublês ----------

type eventosFalsos struct {
	registro Registro
	err      error
	linhas   []string
}

func (e *eventosFalsos) Registrar(_ context.Context, servidor, linha string) (Registro, error) {
	e.linhas = append(e.linhas, linha)
	if e.err != nil {
		return Registro{}, e.err
	}
	registro := e.registro
	registro.Servidor = servidor
	registro.Linha = linha
	if registro.UUID == uuid.Nil {
		registro.UUID = uuid.New()
	}
	return registro, nil
}

type jogadoresFalsos struct {
	presenca   Presenca
	err        error
	chamadas   []string
	derrubados int64
}

func (j *jogadoresFalsos) Entrou(_ context.Context, _, nome, _ string, _ time.Time) (Presenca, error) {
	j.chamadas = append(j.chamadas, "entrou:"+nome)
	return j.presenca, j.err
}

func (j *jogadoresFalsos) SaiuPorZDOID(_ context.Context, _, zdoid string, _ time.Time) (Presenca, error) {
	j.chamadas = append(j.chamadas, "saiu_zdoid:"+zdoid)
	return j.presenca, j.err
}

func (j *jogadoresFalsos) SaiuPorNome(_ context.Context, _, nome string, _ time.Time) (Presenca, error) {
	j.chamadas = append(j.chamadas, "saiu_nome:"+nome)
	return j.presenca, j.err
}

func (j *jogadoresFalsos) Morreu(_ context.Context, _, nome string, _ time.Time) error {
	j.chamadas = append(j.chamadas, "morreu:"+nome)
	return j.err
}

func (j *jogadoresFalsos) Atividade(_ context.Context, _, nome string, _ time.Time) error {
	j.chamadas = append(j.chamadas, "atividade:"+nome)
	return j.err
}

func (j *jogadoresFalsos) ServidorReiniciou(_ context.Context, _ string, _ time.Time) (int64, error) {
	j.chamadas = append(j.chamadas, "reiniciou")
	return j.derrubados, j.err
}

type politicaFalsa struct{ tipos []string }

func (p politicaFalsa) Notificaveis(context.Context) ([]string, error) { return p.tipos, nil }

// montar cria o serviço com os dublês e o barramento de verdade (que é código
// nosso, roda em memória e não tem por que ser dublado).
func montar(t *testing.T, registro Registro, presenca Presenca) (Service, *jogadoresFalsos, *tempo_real.Hub) {
	t.Helper()

	jogadores := &jogadoresFalsos{presenca: presenca}
	hub := tempo_real.NovoHub(8)
	t.Cleanup(hub.Fechar)

	service := NewService(Dependencias{
		Eventos:   &eventosFalsos{registro: registro},
		Jogadores: jogadores,
		Politica:  politicaFalsa{tipos: []string{tipoEntrou, tipoSaiu, tipoMorreu, tipoMensagem}},
		Hub:       hub,
		Servidor:  "playground",
	})
	return service, jogadores, hub
}

// ---------- testes ----------

// TestRegistrarLevaOEventoAoEstadoDoPersonagem confere o roteamento: cada tipo
// de evento chama o método certo do subdomínio de personagens.
func TestRegistrarLevaOEventoAoEstadoDoPersonagem(t *testing.T) {
	casos := []struct {
		nome     string
		registro Registro
		esperada string
	}{
		{
			nome:     "entrada",
			registro: Registro{Tipo: tipoEntrou, Jogador: "Odin", ZDOID: "42:1"},
			esperada: "entrou:Odin",
		},
		{
			nome:     "saída resolve pelo objeto, não pelo nome",
			registro: Registro{Tipo: tipoSaiu, ZDOID: "42:1"},
			esperada: "saiu_zdoid:42:1",
		},
		{
			nome:     "morte",
			registro: Registro{Tipo: tipoMorreu, Jogador: "Odin"},
			esperada: "morreu:Odin",
		},
		{
			nome:     "fala no chat conta como atividade",
			registro: Registro{Tipo: tipoMensagem, Jogador: "Odin", Texto: "vem pro barco"},
			esperada: "atividade:Odin",
		},
		{
			nome:     "servidor no ar derruba todo mundo",
			registro: Registro{Tipo: tipoServidorPronto},
			esperada: "reiniciou",
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			service, jogadores, _ := montar(t, caso.registro, Presenca{Nome: "Odin", Online: true, Mudou: true})

			_, err := service.Registrar(context.Background(), "linha qualquer")

			require.NoError(t, err)
			assert.Equal(t, []string{caso.esperada}, jogadores.chamadas)
		})
	}
}

// TestRegistrarSoNotificaQuandoAPresencaMUDA é a regra que existe por causa do
// jogo: o servidor emite `Got character ZDOID` também no RENASCIMENTO, e sem
// esta trava o canal do Discord receberia "Fulano entrou" a cada morte.
func TestRegistrarSoNotificaQuandoAPresencaMuda(t *testing.T) {
	t.Run("entrada de verdade é notificável", func(t *testing.T) {
		service, _, _ := montar(t,
			Registro{Tipo: tipoEntrou, Jogador: "Odin", ZDOID: "42:1"},
			Presenca{Nome: "Odin", Online: true, Mudou: true})

		resultado, err := service.Registrar(context.Background(), "linha")

		require.NoError(t, err)
		assert.True(t, resultado.Presenca.Mudou)
	})

	t.Run("renascimento não é entrada nova", func(t *testing.T) {
		service, _, _ := montar(t,
			Registro{Tipo: tipoEntrou, Jogador: "Odin", ZDOID: "42:2"},
			Presenca{Nome: "Odin", Online: true, Mudou: false})

		resultado, err := service.Registrar(context.Background(), "linha")

		require.NoError(t, err)
		assert.False(t, resultado.Presenca.Mudou)
		// Sem publicador de Discord configurado no teste, `Notificado` é falso
		// de qualquer forma — o que este teste fixa é a informação que a
		// decisão usa.
		assert.False(t, resultado.Notificado)
	})
}

// TestRegistrarPublicaNoBarramentoDoPainel confere que quem está com a tela
// aberta recebe o evento — inclusive os que não viram mensagem no Discord.
func TestRegistrarPublicaNoBarramentoDoPainel(t *testing.T) {
	service, _, hub := montar(t,
		Registro{Tipo: tipoEntrou, Jogador: "Odin", ZDOID: "42:1", Rotulo: "entrou"},
		Presenca{Nome: "Odin", Online: true, Mudou: true})

	assinante := hub.Assinar()
	defer hub.Cancelar(assinante)

	_, err := service.Registrar(context.Background(), "linha")
	require.NoError(t, err)

	select {
	case mensagem := <-assinante.Canal():
		assert.Equal(t, EventoSSE, mensagem.Nome)
		assert.Contains(t, string(mensagem.Dados), `"jogador":"Odin"`)
	case <-time.After(time.Second):
		t.Fatal("o evento não chegou ao barramento do painel")
	}
}

// TestRegistrarFalhaDoPersonagemNaoPerdeOEvento é a hierarquia que organiza o
// caso de uso: o diário é a etapa que não pode falhar; o saldo é derivado e se
// corrige no próximo evento.
func TestRegistrarFalhaDoPersonagemNaoPerdeOEvento(t *testing.T) {
	jogadores := &jogadoresFalsos{err: ErrPresencaDesconhecida}
	service := NewService(Dependencias{
		Eventos:   &eventosFalsos{registro: Registro{Tipo: tipoSaiu, ZDOID: "99:1"}},
		Jogadores: jogadores,
		Politica:  politicaFalsa{tipos: []string{tipoSaiu}},
		Servidor:  "playground",
	})

	resultado, err := service.Registrar(context.Background(), "linha")

	require.NoError(t, err, "a saída de um objeto órfão não é falha da ingestão")
	assert.Equal(t, tipoSaiu, resultado.Registro.Tipo)
}

// TestRegistrarLoteRecusaCorpoVazioEExcesso cobre os dois limites da porta de
// entrada.
func TestRegistrarLoteRecusaCorpoVazioEExcesso(t *testing.T) {
	service, _, _ := montar(t, Registro{Tipo: tipoDesconhecido()}, Presenca{})

	t.Run("lote só com linhas em branco", func(t *testing.T) {
		_, err := service.RegistrarLote(context.Background(), []string{"", "   "})
		assert.ErrorIs(t, err, ErrCorpoVazio)
	})

	t.Run("lote acima do teto", func(t *testing.T) {
		linhas := make([]string, MaxLinhasPorLote+1)
		for i := range linhas {
			linhas[i] = "Game server connected"
		}
		_, err := service.RegistrarLote(context.Background(), linhas)
		assert.ErrorIs(t, err, ErrLoteGrande)
	})
}

// tipoDesconhecido evita repetir a string literal nos testes que não se
// importam com o tipo do evento.
func tipoDesconhecido() string { return "desconhecido" }
