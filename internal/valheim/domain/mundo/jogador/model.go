// Package jogador guarda o ESTADO de cada personagem do servidor — quem está
// online agora, desde quando, quantas vezes entrou, quantas vezes morreu e
// quanto tempo somou no mundo.
//
// Entidade: Jogador. Dependências: Postgres (repository).
//
// A diferença para o subdomínio `eventos` é a razão de os dois existirem:
// `eventos` é o DIÁRIO (uma linha por acontecimento, nunca alterada) e
// `jogador` é o SALDO (uma linha por pessoa, reescrita a cada acontecimento).
// Quem pergunta "o que houve às 22h31" lê o diário; quem pergunta "quem está
// no servidor agora" lê o saldo — e nenhuma das duas perguntas se responde bem
// com a tabela da outra.
package jogador

import (
	"time"

	"github.com/google/uuid"
)

// Identidade do subdomínio (observador de erros e logs de boot).
const (
	Sistema    = "valheim"
	Dominio    = "mundo"
	Subdominio = "jogador"
)

// Jogador é o estado atual de um personagem no servidor.
type Jogador struct {
	UUID     uuid.UUID `gorm:"column:uuid;type:uuid;primaryKey" json:"uuid"`
	Servidor string    `gorm:"column:servidor;not null" json:"servidor"`
	// Nome é o nome do personagem — a única identidade que o log oferece.
	// Junto com `Servidor`, é a chave natural da tabela.
	Nome string `gorm:"column:nome;not null" json:"nome"`
	// Online é o que o painel mostra em verde. Vira falso na saída e também
	// quando o servidor reinicia (ver Service.ReiniciarServidor).
	Online bool `gorm:"column:online;not null;default:false" json:"online"`
	// ZDOID é o identificador do objeto do personagem na sessão atual — é ele
	// que liga a linha de saída, que não traz nome, a esta pessoa.
	ZDOID string `gorm:"column:zdoid;not null;default:''" json:"zdoid,omitempty"`
	// EntrouEm é o começo da sessão em curso. Nulo quando offline.
	EntrouEm *time.Time `gorm:"column:entrou_em" json:"entrou_em,omitempty"`
	// PrimeiroEm é a primeira vez que este personagem apareceu.
	PrimeiroEm time.Time `gorm:"column:primeiro_em;not null" json:"primeiro_em"`
	// UltimoEm é o último sinal de vida (entrada, fala, morte ou saída).
	UltimoEm time.Time `gorm:"column:ultimo_em;not null" json:"ultimo_em"`
	// Sessoes conta quantas vezes entrou no mundo.
	Sessoes int64 `gorm:"column:sessoes;not null;default:0" json:"sessoes"`
	// Mortes conta os ZDOID zerados.
	Mortes int64 `gorm:"column:mortes;not null;default:0" json:"mortes"`
	// TempoTotalSeg é a soma das sessões JÁ ENCERRADAS. A sessão em curso não
	// entra aqui — quem exibe soma `now - entrou_em` na hora, e é por isso que
	// o número nunca precisa de um job para ficar correto.
	TempoTotalSeg int64 `gorm:"column:tempo_total_seg;not null;default:0" json:"tempo_total_seg"`

	CriadoEm     time.Time `gorm:"column:criado_em;autoCreateTime" json:"criado_em"`
	AtualizadoEm time.Time `gorm:"column:atualizado_em;autoUpdateTime" json:"atualizado_em"`
}

// TableName fixa o nome da tabela no padrão `{sistema}_{dominio}_{subdominio}`.
func (Jogador) TableName() string { return "valheim_mundo_jogador" }

// TempoTotal devolve o tempo somado incluindo a sessão em curso.
func (j Jogador) TempoTotal(agora time.Time) time.Duration {
	total := time.Duration(j.TempoTotalSeg) * time.Second
	if j.Online && j.EntrouEm != nil {
		if emCurso := agora.Sub(*j.EntrouEm); emCurso > 0 {
			total += emCurso
		}
	}
	return total
}

// ListFilter são os filtros da listagem. Campo zerado não filtra.
type ListFilter struct {
	Servidor string
	// ApenasOnline restringe a quem está no mundo agora.
	ApenasOnline bool
	// Nome casa com pedaço do nome (busca do painel).
	Nome string
}
