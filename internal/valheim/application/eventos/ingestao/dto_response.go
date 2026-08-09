package ingestao

import (
	"time"

	"github.com/google/uuid"
)

// EventoAceitoDto é o que o `POST /eventos` devolve por linha recebida.
//
// A resposta é pequena de propósito: quem chama é um `curl` dentro de um
// container, e o que ele precisa saber é "chegou, e virou o quê". O corpo
// inteiro do evento fica na API de consulta.
type EventoAceitoDto struct {
	UUID       uuid.UUID `json:"uuid"`
	Tipo       string    `json:"tipo"`
	Jogador    string    `json:"jogador,omitempty"`
	OcorridoEm time.Time `json:"ocorrido_em"`
	Notificado bool      `json:"notificado"`
}

// IngestaoResponseDto é o corpo de `POST /eventos`.
type IngestaoResponseDto struct {
	Recebidos int `json:"recebidos"`
	// Ignorados são as linhas entendidas que NÃO viraram evento novo — as
	// repetições da mesma saída. Não são erro, e aparecem separadas para quem
	// depura o hook enxergar a diferença entre "não chegou" e "chegou de novo".
	Ignorados int               `json:"ignorados"`
	Eventos   []EventoAceitoDto `json:"eventos"`
}

// EventoAoVivoDto é o que o painel recebe pelo SSE a cada evento novo.
//
// Ele carrega o suficiente para a tela desenhar a linha do feed SEM buscar
// nada: tipo, rótulo, quem, quando e o texto. O que não vai é a linha crua,
// que só interessa a quem abre o detalhe.
type EventoAoVivoDto struct {
	UUID       uuid.UUID `json:"uuid"`
	Servidor   string    `json:"servidor"`
	Tipo       string    `json:"tipo"`
	Rotulo     string    `json:"rotulo"`
	Jogador    string    `json:"jogador,omitempty"`
	Texto      string    `json:"texto,omitempty"`
	OcorridoEm time.Time `json:"ocorrido_em"`
	// Online é a presença DEPOIS do evento — é o que permite ao painel acender
	// e apagar o nome na lista de quem está no mundo sem recarregar a lista.
	Online bool `json:"online"`
	// MudouPresenca diz se este evento mudou o estado (entrou/saiu de verdade).
	MudouPresenca bool `json:"mudou_presenca"`
}

// EventoSSE é o nome do evento no fluxo SSE.
const EventoSSE = "evento"

// paraAoVivo monta o corpo publicado no barramento do painel.
func paraAoVivo(r Registro, p Presenca) EventoAoVivoDto {
	return EventoAoVivoDto{
		UUID:          r.UUID,
		Servidor:      r.Servidor,
		Tipo:          r.Tipo,
		Rotulo:        r.Rotulo,
		Jogador:       primeiroNaoVazio(r.Jogador, p.Nome),
		Texto:         r.Texto,
		OcorridoEm:    r.OcorridoEm,
		Online:        p.Online,
		MudouPresenca: p.Mudou,
	}
}

// primeiroNaoVazio devolve o primeiro texto preenchido.
//
// Serve ao caso da SAÍDA: o evento não traz nome (a linha do servidor só tem o
// ZDOID), mas a presença resolvida traz.
func primeiroNaoVazio(valores ...string) string {
	for _, valor := range valores {
		if valor != "" {
			return valor
		}
	}
	return ""
}
