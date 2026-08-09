package trilhas

import "time"

// LinhaResponseDto é uma linha de log como o painel a recebe.
type LinhaResponseDto struct {
	Seq       int64             `json:"seq"`
	Ts        time.Time         `json:"ts"`
	Nivel     string            `json:"nivel"`
	Mensagem  string            `json:"mensagem"`
	Atributos map[string]string `json:"atributos,omitempty"`
}

// TrilhaResponseDto é o corpo de `GET /observabilidade/logs`.
//
// `ultima_seq` é o que faz a atualização incremental funcionar: o painel guarda
// esse número e o devolve em `depois_de` na chamada seguinte, recebendo só o
// que apareceu no meio-tempo.
type TrilhaResponseDto struct {
	Linhas    []LinhaResponseDto `json:"linhas"`
	UltimaSeq int64              `json:"ultima_seq"`
	Niveis    []string           `json:"niveis"`
}

// ParaResponse monta o corpo da resposta.
func ParaResponse(linhas []Linha, ultimaSeq int64) TrilhaResponseDto {
	dtos := make([]LinhaResponseDto, 0, len(linhas))
	for _, l := range linhas {
		dtos = append(dtos, LinhaResponseDto{
			Seq:       l.Seq,
			Ts:        l.Ts,
			Nivel:     l.Nivel,
			Mensagem:  l.Mensagem,
			Atributos: l.Atributos,
		})
	}
	return TrilhaResponseDto{Linhas: dtos, UltimaSeq: ultimaSeq, Niveis: NiveisAceitos()}
}
