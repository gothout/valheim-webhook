package access_log

import (
	"time"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/pkg/reqctx"
)

// HeaderRequestID é o header de correlação aceito do cliente/proxy. Presente,
// é reaproveitado; ausente, um ray_trace novo é gerado aqui.
const HeaderRequestID = "X-Request-Id"

// Middleware é o primeiro da cadeia: cria o `ray_trace`, injeta no contexto do
// request, devolve no header da resposta e, ao fim, registra a linha de acesso.
//
// O ray_trace volta no header de propósito: quando o dono do servidor abre um
// chamado ("o evento das 22:31 não chegou no Discord"), o que ele tem em mãos é
// a resposta HTTP — e é dali que sai a chave de busca no log.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		inicio := time.Now()

		rayTrace := c.GetHeader(HeaderRequestID)
		if rayTrace == "" {
			rayTrace = reqctx.NovoRayTrace()
		}
		c.Request = c.Request.WithContext(reqctx.ComRayTrace(c.Request.Context(), rayTrace))
		c.Writer.Header().Set(HeaderRequestID, rayTrace)

		c.Next()

		// A rota REGISTRADA, não o caminho concreto: o caminho traria o UUID do
		// recurso para dentro do campo e estouraria a cardinalidade de quem
		// agrega o log. Rota não encontrada não tem FullPath — aí vale o caminho
		// mesmo, que é justamente o que se quer enxergar num 404.
		rota := c.FullPath()
		if rota == "" {
			rota = c.Request.URL.Path
		}

		Registrar(Entrada{
			Ts:        inicio.UTC(),
			Metodo:    c.Request.Method,
			Rota:      rota,
			Status:    c.Writer.Status(),
			DuracaoMs: time.Since(inicio).Milliseconds(),
			Bytes:     c.Writer.Size(),
			IP:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			RayTrace:  rayTrace,
			ErroGin:   c.Errors.ByType(gin.ErrorTypePrivate).String(),
		})
	}
}
