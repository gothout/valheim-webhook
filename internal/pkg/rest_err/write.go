package rest_err

import (
	"context"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/pkg/reqctx"
)

// HeaderRequestID é o header opcional de correlação vindo do cliente ou do
// proxy. Ausente, o ray_trace é gerado.
const HeaderRequestID = "X-Request-Id"

// Observador é o contrato mínimo que o helper de resposta precisa do observador
// de erros. É implementado pelo *errobserve.Observer do subdomínio.
//
// A interface é declarada AQUI, no consumidor: assim `rest_err` não depende de
// `errobserve` e continua sendo uma folha testável com um observador falso.
type Observador interface {
	// ObservarHTTP registra o evento de erro com o status HTTP respondido.
	// A implementação é assíncrona — nunca bloqueia o request.
	ObservarHTTP(ctx context.Context, err error, httpStatus int)
}

// WriteError responde o erro no formato padrão e aborta a cadeia.
//
// Faz três coisas, nessa ordem:
//
//  1. traduz o erro (RestErr passa direto; qualquer outro vira 500 genérico);
//  2. garante o `ray_trace` — do contexto, do header X-Request-Id ou gerado;
//  3. registra o evento no observador do subdomínio e responde.
//
// `obs` nulo é aceito de propósito: subdomínio ainda sem observador, ou boot
// degradado, responde igual — só não alimenta o mapa de erros.
func WriteError(c *gin.Context, obs Observador, err error) {
	restErr := De(err)
	if restErr == nil {
		return
	}

	if restErr.RayTrace == "" {
		restErr.RayTrace = rayTraceDoRequest(c)
	}

	if obs != nil {
		obs.ObservarHTTP(contextoDoRequest(c), restErr.Origem(), restErr.Code)
	}

	c.AbortWithStatusJSON(restErr.Code, restErr)
}

// contextoDoRequest devolve o contexto do request — nunca nulo.
func contextoDoRequest(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}

// rayTraceDoRequest resolve o identificador de correlação nesta ordem: o que o
// middleware já colocou no contexto, senão o header do proxy, senão um novo.
// Nunca devolve vazio — erro sem rastro é erro que não se investiga.
func rayTraceDoRequest(c *gin.Context) string {
	if rt := reqctx.RayTrace(contextoDoRequest(c)); rt != "" {
		return rt
	}
	if c != nil && c.Request != nil {
		if rt := c.GetHeader(HeaderRequestID); rt != "" {
			return rt
		}
	}
	return reqctx.NovoRayTrace()
}
