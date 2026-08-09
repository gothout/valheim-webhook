package jogador

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/iam/middleware"
	"valheim-webhook/internal/pkg/rest_err"
)

// Controller é a borda HTTP do subdomínio.
type Controller interface {
	Routes(routes gin.IRouter)
	List(c *gin.Context)
}

type controllerImpl struct{ service Service }

// NewController monta o controller sobre o serviço.
func NewController(service Service) Controller { return &controllerImpl{service: service} }

// Routes registra as rotas do subdomínio (leitura, com sessão).
func (ctrl *controllerImpl) Routes(routes gin.IRouter) {
	mw := middleware.MustUse().Middleware
	g := routes.Group("/mundo")

	g.GET("/jogadores", mw.Autenticar(), ctrl.List)
}

// List devolve os personagens do servidor.
//
// @Summary      Lista os personagens
// @Tags         Valheim · Mundo
// @Produce      json
// @Param        servidor query string false "Nome do servidor"
// @Param        online   query bool   false "Só quem está no mundo agora"
// @Param        nome     query string false "Trecho do nome"
// @Success      200 {array} jogador.JogadorResponseDto
// @Router       /api/domain/mundo/jogadores [get]
func (ctrl *controllerImpl) List(c *gin.Context) {
	filtro := ListFilter{
		Servidor:     strings.TrimSpace(c.Query("servidor")),
		Nome:         strings.TrimSpace(c.Query("nome")),
		ApenasOnline: verdadeiro(c.Query("online")),
	}

	jogadores, _, err := ctrl.service.List(c.Request.Context(), filtro)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	// A listagem não é paginada de propósito (ver o comentário do repositório):
	// a resposta é o quadro completo de quem já passou pelo servidor.
	c.JSON(http.StatusOK, ParaResponseLista(jogadores, time.Now().UTC()))
}

// verdadeiro aceita as formas que um cliente HTTP escrito à mão costuma usar.
func verdadeiro(valor string) bool {
	switch strings.ToLower(strings.TrimSpace(valor)) {
	case "1", "true", "sim", "on":
		return true
	default:
		return false
	}
}

// responderErro traduz a sentinela do service no status HTTP.
func (ctrl *controllerImpl) responderErro(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		rest_err.WriteError(c, obs, rest_err.NewNotFoundError("Personagem não encontrado.").ComCausa(err))
	default:
		rest_err.WriteError(c, obs, err)
	}
}
