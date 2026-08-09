package tempo_real

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// IntervaloDeBatida é o silêncio máximo do fluxo SSE.
//
// Sem ele, um servidor de Valheim vazio às 4h da manhã não emite evento nenhum
// por horas, e proxies e NATs pelo caminho fecham a conexão por ociosidade — o
// navegador reconecta, mas o painel pisca. Um comentário SSE (`: ping`) a cada
// 25s mantém o cano vivo sem virar evento nenhum na tela.
const IntervaloDeBatida = 25 * time.Second

// EventoDeAbertura é o primeiro evento de toda conexão: serve para o painel
// saber que o fluxo está de pé, mesmo antes de qualquer coisa acontecer no
// servidor de Valheim.
const EventoDeAbertura = "conectado"

// Handler serve o fluxo SSE do painel.
//
// Fica aqui, e não no controller, porque não tem regra de negócio nenhuma: é
// transporte. O controller do subdomínio de aplicação registra a rota e passa
// o hub — assim a rota continua declarada onde as outras estão.
func Handler(h *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		if h == nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}

		cabecalho := c.Writer.Header()
		cabecalho.Set("Content-Type", "text/event-stream")
		cabecalho.Set("Cache-Control", "no-cache")
		cabecalho.Set("Connection", "keep-alive")
		// Desliga o buffer do nginx: sem isto, um proxy reverso segura os
		// eventos para "otimizar" a resposta e o painel só recebe em blocos.
		cabecalho.Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Flush()

		assinante := h.Assinar()
		defer h.Cancelar(assinante)

		// Retry do próprio protocolo: se a conexão cair, o navegador volta em
		// 3s sem que o painel precise de uma linha de JavaScript para isso.
		fmt.Fprint(c.Writer, "retry: 3000\n\n")
		escrever(c, EventoDeAbertura, []byte(`{"ok":true}`))

		batida := time.NewTicker(IntervaloDeBatida)
		defer batida.Stop()

		fim := c.Request.Context().Done()
		for {
			select {
			case <-fim:
				return
			case msg, aberto := <-assinante.Canal():
				if !aberto {
					return // hub encerrado (shutdown do processo)
				}
				escrever(c, msg.Nome, msg.Dados)
			case <-batida.C:
				fmt.Fprint(c.Writer, ": ping\n\n")
				c.Writer.Flush()
			}
		}
	}
}

// escrever emite um evento SSE e empurra para a rede na hora.
//
// `dados` é sempre JSON serializado pelo hub, que nunca contém quebra de linha
// crua — é o que permite escrever o corpo em um único campo `data:`.
func escrever(c *gin.Context, nome string, dados []byte) {
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", nome, dados)
	c.Writer.Flush()
}
