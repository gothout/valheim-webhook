package routes

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/valheim/application/eventos/ingestao"
	"valheim-webhook/internal/valheim/domain/configuracao/integracao"
	"valheim-webhook/internal/valheim/domain/eventos/evento"
	"valheim-webhook/internal/valheim/domain/mundo/jogador"
	"valheim-webhook/internal/valheim/domain/observabilidade/trilhas"
)

// RegisterValheimRoutes registra as rotas do sistema `valheim`.
//
// A vertente não aparece na URL: quem identifica o sistema é o domínio
// (`/eventos`, `/mundo`, `/configuracao`, `/observabilidade`) e a tag da
// documentação.
func RegisterValheimRoutes(domain, application *gin.RouterGroup, raiz gin.IRouter) {
	registrarRotas(domain, "valheim/eventos/evento", evento.Use)
	registrarRotas(domain, "valheim/mundo/jogador", jogador.Use)
	registrarRotas(domain, "valheim/configuracao/integracao", integracao.Use)
	registrarRotas(domain, "valheim/observabilidade/trilhas", trilhas.Use)

	// A ingestão (o caso de uso que costura evento + personagem + Discord +
	// painel) registra rotas em DOIS lugares: o fluxo SSE sob `/api/application`
	// e a porta de entrada `POST /eventos` na RAIZ.
	registrarRotas(application, "valheim/eventos/ingestao", ingestao.Use)
	registrarRotasRaiz(raiz)
}

// registrarRotasRaiz pendura o `POST /eventos`.
//
// Ele não passa pelo `registrarRotas` genérico porque a interface `Roteavel`
// descreve só o `Routes(gin.IRouter)`; a rota da raiz tem um método próprio, e
// alargar a interface por causa de um caso obrigaria todos os outros
// controllers a terem um método que não usam.
func registrarRotasRaiz(raiz gin.IRouter) {
	ctrl, err := ingestao.Use()
	if err != nil {
		slog.Error("[ROUTES] POST "+ingestao.RotaEventos+" NÃO registrado: "+
			"o servidor de Valheim não conseguirá entregar eventos",
			"motivo", err, "conferir", "cmd/bootstrap/domain_init.go")
		return
	}
	ctrl.RotasRaiz(raiz)
}
