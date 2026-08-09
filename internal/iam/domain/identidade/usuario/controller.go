package usuario

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"valheim-webhook/internal/iam/middleware"
	"valheim-webhook/internal/pkg/reqctx"
	"valheim-webhook/internal/pkg/rest_err"
)

// Controller é a borda HTTP do subdomínio.
type Controller interface {
	Routes(routes gin.IRouter)
	List(c *gin.Context)
	Create(c *gin.Context)
	Read(c *gin.Context)
	Update(c *gin.Context)
	TrocarSenha(c *gin.Context)
	Delete(c *gin.Context)
}

type controllerImpl struct{ service Service }

// NewController monta o controller sobre o serviço.
func NewController(service Service) Controller { return &controllerImpl{service: service} }

// Routes registra as rotas do subdomínio.
//
// TODAS exigem papel de administrador — gestão de usuários é o próprio
// significado de administrar esta instalação. A exceção que poderia existir
// ("cada um troca a própria senha") mora no subdomínio de aplicação `auth`,
// junto com o resto do que a pessoa faz sobre si mesma.
func (ctrl *controllerImpl) Routes(routes gin.IRouter) {
	mw := middleware.MustUse().Middleware
	g := routes.Group("/identidade/usuarios")

	g.GET("", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.List)
	g.POST("", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.Create)
	g.GET("/:uuid", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.Read)
	g.PATCH("/:uuid", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.Update)
	g.PATCH("/:uuid/senha", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.TrocarSenha)
	g.DELETE("/:uuid", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.Delete)
}

// List devolve todos os usuários.
//
// @Summary  Lista os usuários do painel
// @Tags     IAM · Usuários
// @Produce  json
// @Success  200 {array} usuario.UsuarioResponseDto
// @Router   /api/domain/identidade/usuarios [get]
func (ctrl *controllerImpl) List(c *gin.Context) {
	usuarios, err := ctrl.service.Listar(c.Request.Context())
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, ParaResponseLista(usuarios))
}

// Create cadastra um usuário.
//
// @Summary  Cria um usuário
// @Tags     IAM · Usuários
// @Accept   json
// @Produce  json
// @Param    corpo body usuario.CriarUsuarioRequestDto true "Dados do usuário"
// @Success  201 {object} usuario.UsuarioResponseDto
// @Router   /api/domain/identidade/usuarios [post]
func (ctrl *controllerImpl) Create(c *gin.Context) {
	var req CriarUsuarioRequestDto
	if err := c.ShouldBindJSON(&req); err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Informe nome, e-mail e senha.").ComCausa(err))
		return
	}

	p, err := req.PapelEscolhido()
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	u, err := ctrl.service.Criar(c.Request.Context(), req.Nome, req.Email, req.Senha, p)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusCreated, ParaResponse(*u))
}

// Read devolve um usuário.
//
// @Summary  Detalha um usuário
// @Tags     IAM · Usuários
// @Produce  json
// @Param    uuid path string true "UUID do usuário"
// @Success  200 {object} usuario.UsuarioResponseDto
// @Router   /api/domain/identidade/usuarios/{uuid} [get]
func (ctrl *controllerImpl) Read(c *gin.Context) {
	id, ok := ctrl.identificador(c)
	if !ok {
		return
	}

	u, err := ctrl.service.Ler(c.Request.Context(), id)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, ParaResponse(*u))
}

// Update muda nome, papel e situação.
//
// @Summary  Atualiza um usuário
// @Tags     IAM · Usuários
// @Accept   json
// @Produce  json
// @Param    uuid  path string                              true "UUID do usuário"
// @Param    corpo body usuario.AtualizarUsuarioRequestDto  true "Dados"
// @Success  200 {object} usuario.UsuarioResponseDto
// @Router   /api/domain/identidade/usuarios/{uuid} [patch]
func (ctrl *controllerImpl) Update(c *gin.Context) {
	id, ok := ctrl.identificador(c)
	if !ok {
		return
	}

	var req AtualizarUsuarioRequestDto
	if err := c.ShouldBindJSON(&req); err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Informe nome e papel.").ComCausa(err))
		return
	}

	p, err := req.PapelEscolhido()
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	atual, err := ctrl.service.Ler(c.Request.Context(), id)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	u, err := ctrl.service.Atualizar(c.Request.Context(), id, req.Nome, p, req.AtivoOu(atual.Ativo))
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, ParaResponse(*u))
}

// TrocarSenha define uma senha nova para o usuário.
//
// @Summary  Troca a senha de um usuário
// @Tags     IAM · Usuários
// @Accept   json
// @Param    uuid  path string                          true "UUID do usuário"
// @Param    corpo body usuario.TrocarSenhaRequestDto   true "Senha nova"
// @Success  204
// @Router   /api/domain/identidade/usuarios/{uuid}/senha [patch]
func (ctrl *controllerImpl) TrocarSenha(c *gin.Context) {
	id, ok := ctrl.identificador(c)
	if !ok {
		return
	}

	var req TrocarSenhaRequestDto
	if err := c.ShouldBindJSON(&req); err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError("Informe a senha.").ComCausa(err))
		return
	}

	if err := ctrl.service.TrocarSenha(c.Request.Context(), id, req.Senha); err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Delete remove um usuário.
//
// @Summary  Remove um usuário
// @Tags     IAM · Usuários
// @Param    uuid path string true "UUID do usuário"
// @Success  204
// @Failure  422 {object} rest_err.RestErr "Último administrador ou a própria conta"
// @Router   /api/domain/identidade/usuarios/{uuid} [delete]
func (ctrl *controllerImpl) Delete(c *gin.Context) {
	id, ok := ctrl.identificador(c)
	if !ok {
		return
	}

	autor := reqctx.Do(c.Request.Context()).UsuarioUUID
	if err := ctrl.service.Remover(c.Request.Context(), id, autor); err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// identificador lê o UUID da rota, respondendo 400 quando não é um.
func (ctrl *controllerImpl) identificador(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError("UUID inválido.").ComCausa(err))
		return uuid.Nil, false
	}
	return id, true
}

// responderErro traduz a sentinela do service no status HTTP.
func (ctrl *controllerImpl) responderErro(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		rest_err.WriteError(c, obs, rest_err.NewNotFoundError("Usuário não encontrado.").ComCausa(err))
	case errors.Is(err, ErrEmailEmUso):
		rest_err.WriteError(c, obs, rest_err.NewUnprocessableEntityError(
			"Já existe um usuário com este e-mail.").ComCausa(err))
	case errors.Is(err, ErrEmailInvalido):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError("E-mail inválido.").ComCausa(err))
	case errors.Is(err, ErrNomeVazio):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError("Informe o nome.").ComCausa(err))
	case errors.Is(err, ErrSenhaFraca):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"A senha precisa ter entre 8 e 72 caracteres.").ComCausa(err))
	case errors.Is(err, ErrPapelInvalido):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Papel inválido: use `admin` ou `visualizador`.").ComCausa(err))
	case errors.Is(err, ErrUltimoAdmin):
		rest_err.WriteError(c, obs, rest_err.NewUnprocessableEntityError(
			"Este é o último administrador ativo: promova outra pessoa antes.").ComCausa(err))
	case errors.Is(err, ErrAutoRemocao):
		rest_err.WriteError(c, obs, rest_err.NewUnprocessableEntityError(
			"Você não pode remover a própria conta.").ComCausa(err))
	default:
		rest_err.WriteError(c, obs, err)
	}
}
