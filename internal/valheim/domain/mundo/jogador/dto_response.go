package jogador

import (
	"time"

	"github.com/google/uuid"
)

// JogadorResponseDto é o personagem como a API o devolve.
type JogadorResponseDto struct {
	UUID       uuid.UUID  `json:"uuid"`
	Servidor   string     `json:"servidor"`
	Nome       string     `json:"nome"`
	Online     bool       `json:"online"`
	EntrouEm   *time.Time `json:"entrou_em,omitempty"`
	PrimeiroEm time.Time  `json:"primeiro_em"`
	UltimoEm   time.Time  `json:"ultimo_em"`
	Sessoes    int64      `json:"sessoes"`
	Mortes     int64      `json:"mortes"`
	// TempoTotalSeg JÁ INCLUI a sessão em curso — é o número que o painel
	// mostra, e ele precisa crescer enquanto a pessoa joga.
	TempoTotalSeg int64 `json:"tempo_total_seg"`
}

// ParaResponse converte a entidade, somando a sessão aberta.
func ParaResponse(j Jogador, agora time.Time) JogadorResponseDto {
	return JogadorResponseDto{
		UUID:          j.UUID,
		Servidor:      j.Servidor,
		Nome:          j.Nome,
		Online:        j.Online,
		EntrouEm:      j.EntrouEm,
		PrimeiroEm:    j.PrimeiroEm,
		UltimoEm:      j.UltimoEm,
		Sessoes:       j.Sessoes,
		Mortes:        j.Mortes,
		TempoTotalSeg: int64(j.TempoTotal(agora).Seconds()),
	}
}

// ParaResponseLista converte a lista inteira com o mesmo instante de
// referência — dois personagens online não podem ter o "agora" diferente na
// mesma resposta.
func ParaResponseLista(jogadores []Jogador, agora time.Time) []JogadorResponseDto {
	dtos := make([]JogadorResponseDto, 0, len(jogadores))
	for _, j := range jogadores {
		dtos = append(dtos, ParaResponse(j, agora))
	}
	return dtos
}
