// Package errobserve é o observador de erros por subdomínio: todo erro
// devolvido por um service ou respondido por um controller vira um evento
// estruturado, formando o mapa de erros do processo — por sistema, domínio,
// subdomínio e código.
//
// Acoplamento de uma linha por subdomínio:
//
//	// singleton.go
//	obs = errobserve.For("valheim", "eventos", "evento", errCodes)
//
//	// service.go — em TODO retorno de erro:
//	return nil, obs.Observe(ctx, ErrLinhaVazia)
//
// `Observe` devolve o erro **intacto**: entra no lugar do `return err` sem
// mudar a assinatura nem o fluxo de quem chama. A emissão é assíncrona — o
// evento é montado no caminho do request (leitura do contexto + `errors.Is` no
// mapa do subdomínio) e entregue aos sinks por uma goroutine própria.
//
// Severidade: sentinela de negócio = warn, erro desconhecido = error, pânico =
// critical.
//
// Pacote folha: importa apenas stdlib e irmãos de `internal/pkg`. Destino novo
// (ClickHouse, webhook, métricas) entra como Sink registrado pelo boot.
package errobserve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"valheim-webhook/internal/pkg/reqctx"
)

// Severidades do evento.
const (
	// SeveridadeWarn — sentinela de negócio conhecida: a regra funcionou. Uma
	// linha de log que não casa com nenhum padrão conhecido é o sistema
	// fazendo seu trabalho, não uma falha.
	SeveridadeWarn = "warn"
	// SeveridadeError — erro não mapeado pelo subdomínio, ou resposta 5xx.
	SeveridadeError = "error"
	// SeveridadeCritical — pânico recuperado. Sempre investigável.
	SeveridadeCritical = "critical"
)

// Códigos reservados do campo `err_code`.
const (
	// CodigoDesconhecido é o destino de todo erro que não casa com nenhuma
	// sentinela do mapa do subdomínio. Volume alto neste código é o sinal de
	// que falta sentinela em `errCodes`.
	CodigoDesconhecido = "ErrUnknown"
	// CodigoPanico marca pânico recuperado — nunca é sentinela de negócio.
	CodigoPanico = "ErrPanic"
)

// MaxMensagem limita o campo `message`. Mensagem de erro carrega dado de
// entrada com frequência (aqui, pedaço de linha de log do servidor) e evento de
// erro é diagnóstico, não dump: 1 KB descreve qualquer falha.
const MaxMensagem = 1024

// MarcaTruncada sinaliza no próprio valor gravado que a mensagem foi cortada.
const MarcaTruncada = "…(truncado)"

// Event é o evento de erro. Quem monta é o Observer; quem consome são os Sinks.
type Event struct {
	Ts         time.Time
	Sistema    string
	Dominio    string
	Subdominio string
	// Funcao é onde o erro foi observado. Vem do stack (a primeira função fora
	// de `errobserve`/`rest_err`) ou é informada, no caso do pânico.
	Funcao string
	// ErrCode é o código estável da sentinela (mapa `errCodes` do subdomínio)
	// ou CodigoDesconhecido. É a chave de agregação do mapa de erros.
	ErrCode string
	Message string
	// Severity é uma das constantes Severidade*.
	Severity string
	// HTTPStatus é o status respondido; zero quando o erro foi observado no
	// service, antes de virar resposta.
	HTTPStatus int
	RayTrace   string
}

// Observer é o observador de um subdomínio: conhece a identidade dele
// (sistema/domínio/subdomínio) e o mapa de sentinelas → códigos.
//
// Todos os métodos são seguros com receptor nulo: subdomínio ainda sem
// observador não derruba nada — o evento sai com a identificação vazia, que é
// justamente o sinal de que faltou o `errobserve.For` no `New`.
type Observer struct {
	sistema    string
	dominio    string
	subdominio string
	errCodes   map[error]string
	agora      func() time.Time
	// despachar é o ponto de entrega. Injetável para os testes exercitarem o
	// Observer sem passar pela fila global.
	despachar func(Event)
}

// For cria o observador do subdomínio.
//
// `errCodes` mapeia sentinela → código estável (`errors.go` do subdomínio) e é
// **copiado**: o mapa do subdomínio nunca muda depois do boot.
func For(sistema, dominio, subdominio string, errCodes map[error]string) *Observer {
	copia := make(map[error]string, len(errCodes))
	for sentinela, codigo := range errCodes {
		if sentinela != nil && codigo != "" {
			copia[sentinela] = codigo
		}
	}
	return &Observer{
		sistema:    sistema,
		dominio:    dominio,
		subdominio: subdominio,
		errCodes:   copia,
		agora:      time.Now,
		despachar:  despachar,
	}
}

// Observe registra o erro e o devolve intacto — é o que permite trocar
// `return err` por `return obs.Observe(ctx, err)` sem mudar mais nada.
//
// Nunca bloqueia: monta o evento e enfileira. `err` nulo não gera evento.
func (o *Observer) Observe(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	o.emitir(o.montar(ctx, funcaoChamadora(), err, 0, ""))
	return err
}

// ObservarHTTP registra o erro já traduzido em resposta, com o status HTTP.
// É o método que `rest_err.WriteError` chama (a interface `rest_err.Observador`
// é satisfeita por este único método).
//
// Status 5xx eleva a severidade para `error` mesmo quando o erro é uma
// sentinela conhecida: falha que virou 500 nunca é "o sistema funcionando".
func (o *Observer) ObservarHTTP(ctx context.Context, err error, httpStatus int) {
	if err == nil {
		return
	}
	o.emitir(o.montar(ctx, funcaoChamadora(), err, httpStatus, ""))
}

// ObservarPanico registra um pânico recuperado com `severity=critical`.
// Chamado pelo middleware de recovery, antes de responder 500.
//
// A função é informada pelo chamador porque, no momento do recover, o stack já
// não aponta para quem entrou em pânico — no HTTP, o valor útil é a rota.
func (o *Observer) ObservarPanico(ctx context.Context, funcao string, recuperado any) {
	ev := o.montar(ctx, funcao, erroDePanico(recuperado), http.StatusInternalServerError, SeveridadeCritical)
	// Pânico nunca herda código de sentinela: o mapa de erros precisa
	// distinguir "regra de negócio negou" de "o processo quebrou".
	ev.ErrCode = CodigoPanico
	o.emitir(ev)
}

// CodigoDe resolve o código estável do erro por `errors.Is` no mapa do
// subdomínio — funciona com erro embrulhado (`fmt.Errorf("...: %w", err)`) e
// com o `RestErr` que encadeia a causa.
//
// Sem correspondência, devolve CodigoDesconhecido. Se mais de uma sentinela
// casar, vence a menor em ordem alfabética: iteração de mapa é aleatória em Go
// e o mesmo erro precisa produzir sempre o mesmo código.
func (o *Observer) CodigoDe(err error) string {
	if err == nil {
		return ""
	}
	if o == nil {
		return CodigoDesconhecido
	}
	escolhido := ""
	for sentinela, codigo := range o.errCodes {
		if errors.Is(err, sentinela) && (escolhido == "" || codigo < escolhido) {
			escolhido = codigo
		}
	}
	if escolhido == "" {
		return CodigoDesconhecido
	}
	return escolhido
}

// Identidade devolve sistema, domínio e subdomínio do observador.
func (o *Observer) Identidade() (string, string, string) {
	if o == nil {
		return "", "", ""
	}
	return o.sistema, o.dominio, o.subdominio
}

// montar transforma erro + contexto no evento pronto.
func (o *Observer) montar(ctx context.Context, funcao string, err error, httpStatus int, severidade string) Event {
	codigo := o.CodigoDe(err)
	if severidade == "" {
		severidade = severidadeDe(codigo, httpStatus)
	}
	agora := time.Now
	if o != nil && o.agora != nil {
		agora = o.agora
	}

	ev := Event{
		Ts:         agora().UTC(),
		Funcao:     funcao,
		ErrCode:    codigo,
		Message:    truncar(mensagemDe(err)),
		Severity:   severidade,
		HTTPStatus: httpStatus,
		RayTrace:   reqctx.RayTrace(ctx),
	}
	if o != nil {
		ev.Sistema, ev.Dominio, ev.Subdominio = o.sistema, o.dominio, o.subdominio
	}
	return ev
}

// emitir entrega o evento ao despachante (global, ou o injetado nos testes).
func (o *Observer) emitir(ev Event) {
	if o == nil || o.despachar == nil {
		despachar(ev)
		return
	}
	o.despachar(ev)
}

// severidadeDe classifica o evento.
func severidadeDe(codigo string, httpStatus int) string {
	if codigo == CodigoDesconhecido || httpStatus >= http.StatusInternalServerError {
		return SeveridadeError
	}
	return SeveridadeWarn
}

// mensagemDe extrai o texto do erro sem nunca devolver vazio.
func mensagemDe(err error) string {
	if err == nil {
		return ""
	}
	if msg := err.Error(); msg != "" {
		return msg
	}
	return fmt.Sprintf("%T sem mensagem", err)
}

// erroDePanico normaliza o valor recuperado em um erro.
func erroDePanico(recuperado any) error {
	if err, ok := recuperado.(error); ok && err != nil {
		return err
	}
	return fmt.Errorf("pânico: %v", recuperado)
}

// truncar corta a mensagem em MaxMensagem bytes sem partir rune ao meio e
// marca o corte.
func truncar(s string) string {
	if len(s) <= MaxMensagem {
		return s
	}
	corte := MaxMensagem
	for corte > 0 && !utf8.ValidString(s[:corte]) {
		corte--
	}
	return s[:corte] + MarcaTruncada
}

// pacotesTransparentes são os pacotes que apenas repassam o erro: as funções
// deles nunca são a resposta útil para "onde isso aconteceu".
var pacotesTransparentes = []string{
	"valheim-webhook/internal/pkg/errobserve.",
	"valheim-webhook/internal/pkg/rest_err.",
}

// funcaoChamadora devolve a primeira função do stack fora dos pacotes
// transparentes — o service que devolveu o erro, ou o controller que respondeu.
//
// Custa uma leitura de stack por erro observado (nunca no caminho de sucesso).
// Sai sem o caminho do pacote e SEM número de linha de propósito: `funcao` é
// chave de leitura do mapa de erros, e a linha muda a cada refatoração.
func funcaoChamadora() string {
	var pcs [16]uintptr
	n := runtime.Callers(2, pcs[:])
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, mais := frames.Next()
		if frame.Function != "" && !transparente(frame.Function) {
			return encurtar(frame.Function)
		}
		if !mais {
			return ""
		}
	}
}

// transparente informa se o frame é de um pacote de repasse.
func transparente(funcao string) bool {
	for _, prefixo := range pacotesTransparentes {
		if strings.HasPrefix(funcao, prefixo) {
			return true
		}
	}
	return false
}

// encurtar reduz `valheim-webhook/internal/.../evento.(*serviceImpl).Registrar`
// para `evento.(*serviceImpl).Registrar`.
func encurtar(funcao string) string {
	if i := strings.LastIndex(funcao, "/"); i >= 0 {
		return funcao[i+1:]
	}
	return funcao
}
