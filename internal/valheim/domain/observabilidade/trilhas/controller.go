package trilhas

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/iam/middleware"
	"valheim-webhook/internal/pkg/rest_err"
)

// Controller é a borda HTTP do subdomínio.
type Controller interface {
	Routes(routes gin.IRouter)
	Listar(c *gin.Context)
}

type controllerImpl struct{ service Service }

// NewController monta o controller sobre o serviço.
func NewController(service Service) Controller { return &controllerImpl{service: service} }

// Routes registra a rota do subdomínio — de ADMINISTRADOR.
//
// O log do processo carrega endereço de banco, nome de host, trecho de
// mensagem de erro e o e-mail de quem entrou. Não é informação de painel
// público, é informação de quem opera a instalação.
func (ctrl *controllerImpl) Routes(routes gin.IRouter) {
	mw := middleware.MustUse().Middleware
	g := routes.Group("/observabilidade")

	g.GET("/logs", mw.Autenticar(), mw.ExigirAdministrador(), ctrl.Listar)
}

// Listar devolve as últimas linhas do log do processo.
//
// @Summary  Lê o log do processo
// @Tags     Valheim · Observabilidade
// @Produce  json
// @Param    nivel     query string false "Nível mínimo (DEBUG|INFO|WARN|ERROR)"
// @Param    busca     query string false "Trecho procurado"
// @Param    depois_de query int    false "Só o que apareceu depois desta sequência"
// @Param    limite    query int    false "Máximo de linhas (teto: 500)"
// @Success  200 {object} trilhas.TrilhaResponseDto
// @Router   /api/domain/observabilidade/logs [get]
func (ctrl *controllerImpl) Listar(c *gin.Context) {
	filtro := Filtro{
		Nivel:    strings.TrimSpace(c.Query("nivel")),
		Busca:    strings.TrimSpace(c.Query("busca")),
		DepoisDe: inteiro(c.Query("depois_de")),
		Limite:   int(inteiro(c.Query("limite"))),
	}

	linhas, ultima, err := ctrl.service.Listar(c.Request.Context(), filtro)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, ParaResponse(linhas, ultima))
}

// inteiro lê um número da query; lixo vira zero (que significa "sem filtro").
func inteiro(bruto string) int64 {
	valor, err := strconv.ParseInt(strings.TrimSpace(bruto), 10, 64)
	if err != nil || valor < 0 {
		return 0
	}
	return valor
}

// responderErro traduz a sentinela do service no status HTTP.
func (ctrl *controllerImpl) responderErro(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNivelInvalido):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Nível inválido: use DEBUG, INFO, WARN ou ERROR.").ComCausa(err))
	case errors.Is(err, ErrFonteIndisponivel):
		rest_err.WriteError(c, obs, rest_err.NewServiceUnavailableError(
			"A trilha de log não está ativa neste processo.").ComCausa(err))
	default:
		rest_err.WriteError(c, obs, err)
	}
}
