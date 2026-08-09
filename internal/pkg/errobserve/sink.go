package errobserve

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Sink é um destino de evento de erro. Plugar um novo destino (banco de logs,
// webhook, métricas) é registrar mais um — nenhum subdomínio muda.
//
// Contrato: `Emit` roda na goroutine do despachante, nunca na do request, e é
// chamado em sequência para cada sink. Sink lento atrasa os eventos seguintes,
// então quem faz I/O deve enfileirar. Pânico dentro de um Emit é contido: não
// derruba o processo nem os outros sinks.
type Sink interface {
	Emit(ev Event)
}

var (
	sinksMu sync.RWMutex
	// O sink do slog é o padrão sempre ativo: o erro aparece no stdout do
	// processo mesmo sem nenhum destino externo configurado.
	sinks = []Sink{SinkSlog{}}
)

// RegisterSink acrescenta um destino. Chamado no boot, uma vez por sink.
// `nil` é ignorado de propósito — é o que permite ao boot registrar um destino
// opcional em uma linha só.
func RegisterSink(s Sink) {
	if s == nil {
		return
	}
	sinksMu.Lock()
	defer sinksMu.Unlock()
	sinks = append(sinks, s)
}

// LimparSinks remove TODOS os destinos, inclusive o do slog. Existe para os
// testes isolarem o que observam — em produção, sink registrado nunca sai.
func LimparSinks() {
	sinksMu.Lock()
	defer sinksMu.Unlock()
	sinks = nil
}

// RestaurarSinksPadrao volta ao estado de boot: só o sink do slog.
func RestaurarSinksPadrao() {
	sinksMu.Lock()
	defer sinksMu.Unlock()
	sinks = []Sink{SinkSlog{}}
}

// sinksAtuais devolve uma cópia da lista — a entrega nunca segura a trava
// enquanto chama código de terceiros.
func sinksAtuais() []Sink {
	sinksMu.RLock()
	defer sinksMu.RUnlock()
	return append([]Sink(nil), sinks...)
}

// TamanhoDaFila é a capacidade do buffer de eventos. Cheia, o evento é
// descartado e contabilizado: observar erro nunca segura um request.
const TamanhoDaFila = 512

// despachante entrega os eventos aos sinks fora do caminho do request.
//
// A goroutine sobe na primeira emissão (processo que nunca erra não paga por
// ela) e vive até Fechar.
type despachante struct {
	fila  chan Event
	sinc  chan chan struct{}
	parar chan struct{}
	fim   chan struct{}

	umaVez      sync.Once
	iniciado    atomic.Bool
	fechado     atomic.Bool
	descartados atomic.Int64
}

// padrao é o despachante do processo.
var padrao = novoDespachante(TamanhoDaFila)

func novoDespachante(tamanho int) *despachante {
	return &despachante{
		fila:  make(chan Event, tamanho),
		sinc:  make(chan chan struct{}),
		parar: make(chan struct{}),
		fim:   make(chan struct{}),
	}
}

// despachar enfileira o evento no despachante do processo.
func despachar(ev Event) { padrao.despachar(ev) }

// Drenar bloqueia até os eventos já enfileirados terem sido entregues aos
// sinks. Serve aos testes ("o evento saiu?") — não é chamado em request nenhum.
func Drenar() { padrao.aguardar() }

// Fechar entrega o que restou na fila e encerra a goroutine. Idempotente.
// Chamado no encerramento do processo. Depois disso, evento observado é
// entregue de forma síncrona — degradado, mas nunca perdido em silêncio.
func Fechar() { padrao.fechar() }

func (d *despachante) despachar(ev Event) {
	if d.fechado.Load() {
		d.entregar(ev)
		return
	}
	d.umaVez.Do(func() {
		d.iniciado.Store(true)
		go d.rodar()
	})

	select {
	case d.fila <- ev:
	default:
		d.descartados.Add(1)
	}
}

// rodar é o laço de entrega.
func (d *despachante) rodar() {
	defer close(d.fim)
	for {
		select {
		case ev := <-d.fila:
			d.entregar(ev)
		case resposta := <-d.sinc:
			// Esvazia o que já estava enfileirado ANTES de responder: é isso que
			// faz Drenar significar "os eventos anteriores já saíram".
			d.drenar()
			close(resposta)
		case <-d.parar:
			d.drenar()
			return
		}
	}
}

// drenar entrega o que estiver na fila, sem bloquear.
func (d *despachante) drenar() {
	for {
		select {
		case ev := <-d.fila:
			d.entregar(ev)
		default:
			return
		}
	}
}

// entregar manda o evento para todos os sinks registrados.
func (d *despachante) entregar(ev Event) {
	if descartados := d.descartados.Swap(0); descartados > 0 {
		slog.Warn("[ERROBSERVE] fila cheia: eventos de erro descartados",
			"descartados", descartados, "capacidade", cap(d.fila))
	}
	for _, sink := range sinksAtuais() {
		emitirComProtecao(sink, ev)
	}
}

// emitirComProtecao isola o sink: um destino com defeito não pode derrubar o
// processo (nem impedir os outros de receberem o evento).
func emitirComProtecao(sink Sink, ev Event) {
	defer func() {
		if recuperado := recover(); recuperado != nil {
			slog.Error("[ERROBSERVE] sink entrou em pânico e foi ignorado",
				"sink", fmt.Sprintf("%T", sink), "panico", fmt.Sprint(recuperado))
		}
	}()
	sink.Emit(ev)
}

func (d *despachante) aguardar() {
	if !d.iniciado.Load() || d.fechado.Load() {
		return // nada enfileirado: ou nunca houve evento, ou a entrega é síncrona
	}
	resposta := make(chan struct{})
	select {
	case d.sinc <- resposta:
		<-resposta
	case <-d.fim:
	}
}

func (d *despachante) fechar() {
	if d.fechado.Swap(true) {
		return
	}
	if !d.iniciado.Load() {
		return // goroutine nunca subiu
	}
	close(d.parar)
	<-d.fim
}

// SinkSlog é o destino sempre ativo: o evento vira uma linha no log do
// processo — é o que aparece no `docker logs` durante o desenvolvimento.
type SinkSlog struct {
	logger *slog.Logger
}

// Emit escreve o evento no log, com nível derivado da severidade.
func (s SinkSlog) Emit(ev Event) {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Log(context.Background(), nivelDe(ev.Severity),
		"[ERRO] "+ev.Sistema+"/"+ev.Dominio+"/"+ev.Subdominio,
		"ts", ev.Ts,
		"funcao", ev.Funcao,
		"err_code", ev.ErrCode,
		"message", ev.Message,
		"severity", ev.Severity,
		"http_status", ev.HTTPStatus,
		"ray_trace", ev.RayTrace,
	)
}

// nivelDe traduz a severidade do evento para o nível do slog.
func nivelDe(severidade string) slog.Level {
	switch severidade {
	case SeveridadeWarn:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}
