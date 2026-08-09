package integracao

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/iam/middleware"
	"valheim-webhook/internal/pkg/reqctx"
	"valheim-webhook/internal/pkg/rest_err"
)

// Controller é a borda HTTP do subdomínio.
type Controller interface {
	Routes(routes gin.IRouter)
	Ler(c *gin.Context)
	Salvar(c *gin.Context)
	Testar(c *gin.Context)
}

type controllerImpl struct{ service Service }

// NewController monta o controller sobre o serviço.
func NewController(service Service) Controller { return &controllerImpl{service: service} }

// Routes registra as rotas do subdomínio — todas de ADMINISTRADOR.
//
// Inclusive a leitura: mesmo mascarada, a configuração diz para qual canal do
// Discord o servidor conversa, e isso é informação de administração.
func (ctrl *controllerImpl) Routes(routes gin.IRouter) {
	mw := middleware.MustUse().Middleware
	g := routes.Group("/configuracao/discord")

	g.GET("", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.Ler)
	g.PUT("", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.Salvar)
	g.POST("/teste", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.Testar)
}

// Ler devolve a configuração vigente, com os segredos mascarados.
//
// @Summary  Lê a integração com o Discord
// @Tags     Valheim · Configuração
// @Produce  json
// @Success  200 {object} integracao.IntegracaoResponseDto
// @Router   /api/domain/configuracao/discord [get]
func (ctrl *controllerImpl) Ler(c *gin.Context) {
	i, err := ctrl.service.Ler(c.Request.Context())
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, ParaResponse(*i, ctrl.service.EventosPadrao()))
}

// Salvar grava a configuração e reconfigura o publicador na hora.
//
// @Summary  Salva a integração com o Discord
// @Tags     Valheim · Configuração
// @Accept   json
// @Produce  json
// @Param    corpo body integracao.SalvarIntegracaoRequestDto true "Configuração"
// @Success  200 {object} integracao.IntegracaoResponseDto
// @Router   /api/domain/configuracao/discord [put]
func (ctrl *controllerImpl) Salvar(c *gin.Context) {
	var req SalvarIntegracaoRequestDto
	if err := c.ShouldBindJSON(&req); err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Informe ao menos o modo (`bot` ou `webhook`).").ComCausa(err))
		return
	}

	autor := reqctx.Do(c.Request.Context()).UsuarioUUID
	i, err := ctrl.service.Salvar(c.Request.Context(), req, autor)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, ParaResponse(*i, ctrl.service.EventosPadrao()))
}

// Testar publica uma mensagem de teste.
//
// Responde 202, e não 200, porque o envio é ASSÍNCRONO: o que esta rota garante
// é que a mensagem entrou na fila com uma configuração válida. Se o Discord
// recusar depois, quem conta é o log — e é isso que a tela diz ao usuário.
//
// @Summary  Envia uma mensagem de teste
// @Tags     Valheim · Configuração
// @Success  202
// @Failure  422 {object} rest_err.RestErr "Integração desligada ou sem destino"
// @Router   /api/domain/configuracao/discord/teste [post]
func (ctrl *controllerImpl) Testar(c *gin.Context) {
	identidade := reqctx.Do(c.Request.Context())

	if err := ctrl.service.Testar(c.Request.Context(), identidade.Nome); err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"mensagem": "Mensagem de teste enfileirada. Confira o canal do Discord.",
	})
}

// responderErro traduz a sentinela do service no status HTTP.
func (ctrl *controllerImpl) responderErro(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrModoInvalido):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Modo inválido: use `bot` ou `webhook`.").ComCausa(err))
	case errors.Is(err, ErrURLInvalida):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"URL de webhook inválida: ela precisa ser a do webhook do canal "+
				"(https://discord.com/api/webhooks/...).").ComCausa(err))
	case errors.Is(err, ErrEventoInvalido):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Há um tipo de evento desconhecido na lista.").ComCausa(err))
	case errors.Is(err, ErrSemDestino):
		rest_err.WriteError(c, obs, rest_err.NewUnprocessableEntityError(
			"Para habilitar, preencha o destino do modo escolhido: "+
				"a URL do webhook, ou o token do bot e o ID do canal.").ComCausa(err))
	case errors.Is(err, ErrNaoConfigurada):
		rest_err.WriteError(c, obs, rest_err.NewUnprocessableEntityError(
			"A integração está desligada ou sem destino: salve a configuração antes de testar.").ComCausa(err))
	case errors.Is(err, ErrTeste):
		rest_err.WriteError(c, obs, rest_err.NewServiceUnavailableError(
			"Não foi possível falar com o Discord agora.").ComCausa(err))
	default:
		rest_err.WriteError(c, obs, err)
	}
}
