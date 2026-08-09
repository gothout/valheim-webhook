package evento

import (
	"time"

	"github.com/google/uuid"
)

// EventoResponseDto é o evento como a API o devolve.
//
// Ele existe separado da entidade por um motivo concreto: a linha CRUA é um
// campo grande e repetitivo, e o painel só a mostra quando o usuário abre o
// detalhe. Mandá-la em toda listagem multiplicaria por três o tamanho da
// resposta do feed.
type EventoResponseDto struct {
	UUID       uuid.UUID `json:"uuid"`
	Servidor   string    `json:"servidor"`
	Tipo       Tipo      `json:"tipo"`
	Rotulo     string    `json:"rotulo"`
	Jogador    string    `json:"jogador,omitempty"`
	SteamID    string    `json:"steam_id,omitempty"`
	ZDOID      string    `json:"zdoid,omitempty"`
	Texto      string    `json:"texto,omitempty"`
	OcorridoEm time.Time `json:"ocorrido_em"`
	CriadoEm   time.Time `json:"criado_em"`
	// Linha só é preenchida no detalhe (`GET /eventos/{uuid}`).
	Linha string `json:"linha,omitempty"`
}

// ParaResponse converte a entidade no DTO de listagem (sem a linha crua).
func ParaResponse(e Evento) EventoResponseDto {
	return EventoResponseDto{
		UUID:       e.UUID,
		Servidor:   e.Servidor,
		Tipo:       e.Tipo,
		Rotulo:     e.Tipo.Rotulo(),
		Jogador:    e.Jogador,
		SteamID:    e.SteamID,
		ZDOID:      e.ZDOID,
		Texto:      e.Texto,
		OcorridoEm: e.OcorridoEm,
		CriadoEm:   e.CriadoEm,
	}
}

// ParaResponseCompleto converte a entidade incluindo a linha crua (detalhe).
func ParaResponseCompleto(e Evento) EventoResponseDto {
	dto := ParaResponse(e)
	dto.Linha = e.Linha
	return dto
}

// ParaResponseLista converte a página inteira.
func ParaResponseLista(eventos []Evento) []EventoResponseDto {
	dtos := make([]EventoResponseDto, 0, len(eventos))
	for _, e := range eventos {
		dtos = append(dtos, ParaResponse(e))
	}
	return dtos
}

// ResumoResponseDto é o corpo de `GET /eventos/resumo`.
type ResumoResponseDto struct {
	Total        int64            `json:"total"`
	PorTipo      map[string]int64 `json:"por_tipo"`
	UltimoEvento *time.Time       `json:"ultimo_evento,omitempty"`
}

// ParaResumoResponse converte o resumo, garantindo os tipos zerados.
func ParaResumoResponse(r Resumo) ResumoResponseDto {
	porTipo := make(map[string]int64, len(TiposConhecidos()))
	for _, tipo := range TiposConhecidos() {
		porTipo[string(tipo)] = r.PorTipo[tipo]
	}
	return ResumoResponseDto{Total: r.Total, PorTipo: porTipo, UltimoEvento: r.UltimoEvento}
}
