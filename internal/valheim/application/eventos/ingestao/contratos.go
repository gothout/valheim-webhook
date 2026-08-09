package ingestao

import (
	"context"
	"time"

	"github.com/google/uuid"

	"valheim-webhook/internal/pkg/tempo_real"
)

// Registro é o evento já gravado, visto por este caso de uso.
//
// Tipo é `string`, e não o tipo nomeado do subdomínio de eventos, de propósito:
// o vocabulário pertence a ELE, e copiar o tipo para cá criaria duas
// definições da mesma coisa. O que atravessa a fronteira é o valor.
type Registro struct {
	UUID       uuid.UUID
	Servidor   string
	Tipo       string
	Rotulo     string
	Jogador    string
	SteamID    string
	ZDOID      string
	Texto      string
	Linha      string
	OcorridoEm time.Time
}

// Presenca é o efeito do evento sobre o estado do personagem.
type Presenca struct {
	Nome   string
	Online bool
	// Mudou distingue "entrou agora" de "renasceu, mas já estava dentro". É o
	// que evita anunciar a mesma chegada três vezes no Discord.
	Mudou bool
}

// Eventos é o subdomínio `eventos/evento` visto por uma fresta.
type Eventos interface {
	// Registrar interpreta a linha e a grava.
	Registrar(ctx context.Context, servidor, linha string) (Registro, error)
}

// Jogadores é o subdomínio `mundo/jogador` visto por uma fresta.
//
// Cada método corresponde a um acontecimento, e não a um CRUD: quem chama não
// decide como o saldo muda, só conta o que houve.
type Jogadores interface {
	Entrou(ctx context.Context, servidor, nome, zdoid string, quando time.Time) (Presenca, error)
	SaiuPorZDOID(ctx context.Context, servidor, zdoid string, quando time.Time) (Presenca, error)
	SaiuPorNome(ctx context.Context, servidor, nome string, quando time.Time) (Presenca, error)
	Morreu(ctx context.Context, servidor, nome string, quando time.Time) error
	Atividade(ctx context.Context, servidor, nome string, quando time.Time) error
	// ServidorReiniciou derruba todo mundo e devolve quantos estavam online.
	ServidorReiniciou(ctx context.Context, servidor string, quando time.Time) (int64, error)
}

// Politica é o subdomínio `configuracao/integracao` visto por uma fresta:
// quais tipos de evento merecem mensagem AGORA (a configuração muda em tempo de
// execução, pelo painel — por isso é uma consulta, e não uma lista fixa no
// boot).
type Politica interface {
	Notificaveis(ctx context.Context) ([]string, error)
}

// Dependencias é o que o boot liga neste subdomínio.
type Dependencias struct {
	Eventos   Eventos
	Jogadores Jogadores
	Politica  Politica
	// Hub é o barramento do painel. Nulo desliga o tempo real sem quebrar nada.
	Hub *tempo_real.Hub
	// Servidor é o nome com que os eventos são carimbados.
	Servidor string
}
