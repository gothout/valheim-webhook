// Package routes monta o gin.Engine da API: middlewares globais, CORS, health
// check, o painel e as três famílias de rotas.
//
// Três, e não duas como no Atila, porque aqui existe uma rota que não pertence
// a nenhuma camada: o `POST /eventos`, na RAIZ. Ela é a porta por onde o
// container do servidor de Valheim entrega as linhas de log, e o endereço dela
// está escrito à mão numa variável de ambiente daquele container.
//
// Regra que vale para todas: o middleware de autorização é declarado ROTA A
// ROTA pelo controller de cada subdomínio — nunca aplicado ao grupo inteiro
// aqui. Assim uma rota pública nova não depende de alguém lembrar de excluí-la
// de um middleware global.
//
// O pacote não importa `internal/infra`: o estado das dependências chega pelas
// Sondas injetadas pelo `cmd/bootstrap`.
package routes

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/pkg/config"
	"valheim-webhook/internal/pkg/errobserve"
	"valheim-webhook/internal/pkg/log/access_log"
	"valheim-webhook/internal/pkg/reqctx"
	"valheim-webhook/internal/pkg/rest_err"
)

// Prefixos das famílias de rotas.
const (
	PrefixoDomain      = "/api/domain"
	PrefixoApplication = "/api/application"
)

// ErrRotaDesconhecida é a sentinela da borda HTTP: request que não casou com
// nenhuma rota. Vira evento `warn` no mapa de erros — volume alto aqui costuma
// ser cliente desatualizado ou varredura, e é isso que se quer enxergar.
var ErrRotaDesconhecida = errors.New("rota não encontrada")

// errCodesHTTP é o mapa sentinela → código da borda HTTP.
var errCodesHTTP = map[error]string{
	ErrRotaDesconhecida: "ErrRotaDesconhecida",
}

// obsHTTP observa os erros que nascem na borda, fora de qualquer subdomínio:
// pânico recuperado e rota desconhecida.
var obsHTTP = errobserve.For("http", "server", "routes", errCodesHTTP)

// Grupos são as famílias de rotas.
type Grupos struct {
	Domain      *gin.RouterGroup
	Application *gin.RouterGroup
	// Raiz é o engine: recebe o `POST /eventos`. Ver o comentário do pacote.
	Raiz gin.IRouter
}

// Opcoes reúne o que o boot injeta no engine além da configuração.
type Opcoes struct {
	// Sondas informam o estado das dependências no GET /api/status.
	Sondas map[string]Sonda
}

// NovoEngine monta o engine completo.
//
// Não mexe no modo do Gin (`gin.SetMode`) de propósito: modo é estado global do
// processo e quem decide é o boot, não quem monta rotas.
func NovoEngine(cfg *config.Config, o Opcoes) (*gin.Engine, error) {
	if cfg == nil {
		return nil, errors.New("configuração nula ao montar o engine HTTP")
	}

	engine := gin.New()

	// Confiar em proxy é decisão explícita: com a lista vazia (o padrão) o
	// ClientIP é o endereço da conexão e nenhum X-Forwarded-For é aceito.
	if err := engine.SetTrustedProxies(cfg.Server.HTTP.TrustedProxy); err != nil {
		return nil, fmt.Errorf("server.http.trusted_proxy inválido: %w", err)
	}

	engine.Use(
		// PRIMEIRO da cadeia: é quem cria o ray_trace que o observador de
		// erros e a resposta de erro reaproveitam.
		access_log.Middleware(),
		recuperarPanico(),
		CORS(cfg),
	)

	// Método errado em rota existente também cai aqui
	// (HandleMethodNotAllowed fica desligado): a API não confirma a existência
	// de um recurso pela diferença entre 404 e 405.
	engine.NoRoute(rotaDesconhecida)

	engine.GET(RotaStatus, statusHandler(cfg, o.Sondas, time.Now().UTC()))

	if err := RegistrarPainel(engine, cfg); err != nil {
		return nil, err
	}

	SetupApiRoutes(engine)
	avisarCORSAberto(cfg)

	return engine, nil
}

// SetupApiRoutes cria os grupos e delega o registro a cada sistema.
func SetupApiRoutes(r *gin.Engine) Grupos {
	g := Grupos{
		Domain:      r.Group(PrefixoDomain),
		Application: r.Group(PrefixoApplication),
		Raiz:        r,
	}

	RegisterIamRoutes(g.Domain, g.Application)
	RegisterValheimRoutes(g.Domain, g.Application, g.Raiz)

	return g
}

// Roteavel é o mínimo que este pacote precisa de um controller: saber
// registrar as próprias rotas. Declarada aqui, no consumidor, para não depender
// do tipo `Controller` de cada subdomínio.
type Roteavel interface {
	Routes(gin.IRouter)
}

// registrarRotas pendura o controller de um subdomínio no grupo da camada.
//
// Recebe o `Use()` do subdomínio, e não o controller pronto, por causa da ordem
// do boot: quem monta o engine num teste não passou pelo `InitDomains`, e um
// `MustUse()` derrubaria o processo. Sem inicialização, o subdomínio fica sem
// rotas e o motivo sai no log.
func registrarRotas[C Roteavel](g gin.IRouter, nome string, use func() (C, error)) {
	ctrl, err := use()
	if err != nil {
		slog.Warn("[ROUTES] subdomínio sem rotas registradas",
			"subdominio", nome, "motivo", err,
			"conferir", "cmd/bootstrap/domain_init.go")
		return
	}
	ctrl.Routes(g)
}

// recuperarPanico transforma pânico em 500 no formato padrão e emite o evento
// `severity=critical`.
//
// O `WriteError` recebe observador nulo de propósito: o evento crítico já foi
// emitido acima, e observar de novo duplicaria a linha no mapa de erros.
func recuperarPanico() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recuperado any) {
		obsHTTP.ObservarPanico(c.Request.Context(), rotaDe(c), recuperado)
		slog.Error("[HTTP] pânico recuperado",
			"metodo", c.Request.Method,
			"path", c.Request.URL.Path,
			"ray_trace", reqctx.RayTrace(c.Request.Context()),
			"panico", fmt.Sprint(recuperado),
		)
		rest_err.WriteError(c, nil, rest_err.NewInternalServerError(
			"Erro interno ao processar a requisição. Informe o ray_trace ao suporte.",
		))
	})
}

// rotaDesconhecida responde 404 no formato padrão, com ray_trace.
func rotaDesconhecida(c *gin.Context) {
	rest_err.WriteError(c, obsHTTP, rest_err.NewNotFoundError("Rota não encontrada.").
		ComCausa(ErrRotaDesconhecida))
}

// rotaDe descreve a rota para o campo `funcao` do evento.
//
// Usa o padrão registrado (`/api/domain/x/:uuid`), nunca o caminho concreto: o
// caminho traria o UUID do recurso para dentro do campo e estouraria a
// cardinalidade da agregação.
func rotaDe(c *gin.Context) string {
	rota := c.FullPath()
	if rota == "" {
		rota = "(rota desconhecida)"
	}
	return c.Request.Method + " " + rota
}
