package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/iam/middleware"
	"valheim-webhook/internal/pkg/reqctx"
	"valheim-webhook/internal/pkg/rest_err"
)

// Controller é a borda HTTP do caso de uso.
type Controller interface {
	Routes(routes gin.IRouter)
	Login(c *gin.Context)
	Logout(c *gin.Context)
	Eu(c *gin.Context)
	TrocarMinhaSenha(c *gin.Context)
}

type controllerImpl struct {
	service Service
	opcoes  Opcoes
}

// NewController monta o controller.
func NewController(service Service, opcoes Opcoes) Controller {
	return &controllerImpl{service: service, opcoes: opcoes}
}

// Routes registra as rotas do caso de uso.
//
// `login` é a ÚNICA rota da API sem sessão — é a porta. As outras três exigem
// `Autenticar()`, e nenhuma exige papel: trocar a própria senha e ver a própria
// conta são coisas que todo usuário faz.
func (ctrl *controllerImpl) Routes(routes gin.IRouter) {
	mw := middleware.MustUse().Middleware
	g := routes.Group("/identidade/auth")

	g.POST("/login", ctrl.Login)
	g.POST("/logout", mw.Autenticar(), ctrl.Logout)
	g.GET("/eu", mw.Autenticar(), ctrl.Eu)
	g.PATCH("/eu/senha", mw.Autenticar(), ctrl.TrocarMinhaSenha)
}

// Login abre a sessão.
//
// @Summary  Entra no painel
// @Tags     IAM · Sessão
// @Accept   json
// @Produce  json
// @Param    corpo body auth.LoginRequestDto true "Credenciais"
// @Success  200 {object} auth.SessaoResponseDto
// @Failure  401 {object} rest_err.RestErr
// @Router   /api/application/identidade/auth/login [post]
func (ctrl *controllerImpl) Login(c *gin.Context) {
	var req LoginRequestDto
	if err := c.ShouldBindJSON(&req); err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Informe e-mail e senha.").ComCausa(err))
		return
	}

	sessao, err := ctrl.service.Entrar(c.Request.Context(), req.Email, req.Senha)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	ctrl.gravarCookie(c, sessao.Token, sessao.ExpiraEm)
	c.JSON(http.StatusOK, SessaoResponseDto{
		Usuario:  ParaContaResponse(sessao.Conta),
		ExpiraEm: sessao.ExpiraEm,
	})
}

// Logout fecha a sessão apagando o cookie.
//
// @Summary  Sai do painel
// @Tags     IAM · Sessão
// @Success  204
// @Router   /api/application/identidade/auth/logout [post]
func (ctrl *controllerImpl) Logout(c *gin.Context) {
	ctrl.apagarCookie(c)
	c.Status(http.StatusNoContent)
}

// Eu devolve a conta da sessão.
//
// @Summary  Quem sou eu
// @Tags     IAM · Sessão
// @Produce  json
// @Success  200 {object} auth.ContaResponseDto
// @Router   /api/application/identidade/auth/eu [get]
func (ctrl *controllerImpl) Eu(c *gin.Context) {
	identidade := reqctx.Do(c.Request.Context())

	conta, err := ctrl.service.Eu(c.Request.Context(), identidade.UsuarioUUID)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, ParaContaResponse(*conta))
}

// TrocarMinhaSenha troca a senha do próprio usuário.
//
// @Summary  Troca a própria senha
// @Tags     IAM · Sessão
// @Accept   json
// @Param    corpo body auth.TrocarMinhaSenhaRequestDto true "Senhas"
// @Success  204
// @Router   /api/application/identidade/auth/eu/senha [patch]
func (ctrl *controllerImpl) TrocarMinhaSenha(c *gin.Context) {
	var req TrocarMinhaSenhaRequestDto
	if err := c.ShouldBindJSON(&req); err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Informe a senha atual e a nova.").ComCausa(err))
		return
	}

	identidade := reqctx.Do(c.Request.Context())
	err := ctrl.service.TrocarMinhaSenha(c.Request.Context(), identidade.UsuarioUUID,
		req.SenhaAtual, req.SenhaNova)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	// A senha mudou: a sessão atual continua válida (é a mesma pessoa, no mesmo
	// navegador), mas o cookie é reemitido com a validade cheia — trocar a
	// senha é o momento em que alguém MENOS espera ser deslogado.
	c.Status(http.StatusNoContent)
}

// gravarCookie põe o token no cookie de sessão.
//
// `HttpOnly` impede que qualquer script da página leia o token; `SameSite=Lax`
// impede que um site de terceiros faça o navegador usar a sessão numa
// requisição escondida (CSRF) sem quebrar o clique num link para o painel.
func (ctrl *controllerImpl) gravarCookie(c *gin.Context, token string, expira time.Time) {
	segundos := int(time.Until(expira).Seconds())
	if segundos < 1 {
		segundos = 1
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.CookieSessao, token, segundos, "/",
		ctrl.opcoes.CookieDominio, ctrl.opcoes.CookieSeguro, true)
}

// apagarCookie remove a sessão do navegador.
//
// O domínio precisa ser o MESMO usado ao gravar: cookie apagado com domínio
// diferente vira um segundo cookie vazio, e o original continua lá — o logout
// pareceria não funcionar.
func (ctrl *controllerImpl) apagarCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.CookieSessao, "", -1, "/",
		ctrl.opcoes.CookieDominio, ctrl.opcoes.CookieSeguro, true)
}

// responderErro traduz a sentinela do service no status HTTP.
func (ctrl *controllerImpl) responderErro(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrCredenciais):
		// O cookie é apagado junto: se havia sessão velha no navegador, ela não
		// deve sobreviver a uma tentativa de login recusada.
		ctrl.apagarCookie(c)
		rest_err.WriteError(c, obs, rest_err.NewUnauthorizedError(
			"E-mail ou senha inválidos.").ComCausa(err))
	case errors.Is(err, ErrContaInativa):
		ctrl.apagarCookie(c)
		rest_err.WriteError(c, obs, rest_err.New(http.StatusForbidden, "forbidden",
			"Conta inativa. Procure um administrador.").ComCausa(err))
	case errors.Is(err, ErrContaDesconhecida), errors.Is(err, ErrSemSessao):
		ctrl.apagarCookie(c)
		rest_err.WriteError(c, obs, rest_err.NewUnauthorizedError("Sessão inválida.").ComCausa(err))
	case errors.Is(err, ErrSenhaFraca):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"A senha precisa ter entre 8 e 72 caracteres.").ComCausa(err))
	case errors.Is(err, ErrIndisponivel):
		rest_err.WriteError(c, obs, rest_err.NewServiceUnavailableError(
			"Não foi possível concluir agora. Tente de novo em instantes.").ComCausa(err))
	default:
		rest_err.WriteError(c, obs, err)
	}
}
