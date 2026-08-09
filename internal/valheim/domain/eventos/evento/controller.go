package evento

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"valheim-webhook/internal/iam/middleware"
	"valheim-webhook/internal/pkg/pagination"
	"valheim-webhook/internal/pkg/rest_err"
)

// Controller é a borda HTTP do subdomínio.
type Controller interface {
	Routes(routes gin.IRouter)
	List(c *gin.Context)
	Read(c *gin.Context)
	Resumo(c *gin.Context)
}

type controllerImpl struct{ service Service }

// NewController monta o controller sobre o serviço.
func NewController(service Service) Controller { return &controllerImpl{service: service} }

// JanelaPadraoDoResumo é o recorte do `GET /resumo` quando `desde` não vem: 24
// horas, que é o que o cabeçalho do painel quer dizer com "hoje no servidor".
const JanelaPadraoDoResumo = 24 * time.Hour

// Routes registra as rotas do subdomínio.
//
// As três são de LEITURA e exigem sessão — qualquer papel serve. Quem escreve
// aqui é a ingestão (`POST /eventos`, na raiz), que não usa sessão de usuário e
// sim o token do servidor de jogo.
//
// As rotas de consulta e a de resumo ficam em prefixos diferentes de propósito
// (`/eventos/eventos/:uuid` e `/eventos/resumo`): rota estática irmã de rota
// com parâmetro é o tipo de vizinhança que já foi armadilha em roteador de Go,
// e evitá-la custa nada.
func (ctrl *controllerImpl) Routes(routes gin.IRouter) {
	mw := middleware.MustUse().Middleware
	g := routes.Group("/eventos")

	g.GET("/eventos", mw.Autenticar(), ctrl.List)
	g.GET("/eventos/:uuid", mw.Autenticar(), ctrl.Read)
	g.GET("/resumo", mw.Autenticar(), ctrl.Resumo)
}

// List devolve a página de eventos que casa com o filtro.
//
// @Summary      Lista os eventos do servidor
// @Tags         Valheim · Eventos
// @Produce      json
// @Param        tipo     query    []string false "Tipos (repetível ou separado por vírgula)"
// @Param        jogador  query    string   false "Nome exato do personagem"
// @Param        desde    query    string   false "Início do recorte (RFC3339)"
// @Param        ate      query    string   false "Fim do recorte (RFC3339)"
// @Param        busca    query    string   false "Trecho procurado na fala ou na linha crua"
// @Param        page     query    int      false "Página"
// @Param        pageSize query    int      false "Tamanho da página (máx. 200)"
// @Success      200 {object} pagination.Response[evento.EventoResponseDto]
// @Router       /api/domain/eventos/eventos [get]
func (ctrl *controllerImpl) List(c *gin.Context) {
	var req ListarRequestDto
	if err := c.ShouldBindQuery(&req); err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError("Filtros inválidos.").ComCausa(err))
		return
	}

	filtro, err := req.ParaFiltro()
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	params := pagination.FromQuery(c)
	eventos, total, err := ctrl.service.List(c.Request.Context(), filtro, params)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	c.JSON(http.StatusOK, pagination.NovaResponse(ParaResponseLista(eventos), params, total))
}

// Read devolve um evento inteiro, com a linha crua.
//
// @Summary      Detalha um evento
// @Tags         Valheim · Eventos
// @Produce      json
// @Param        uuid path string true "UUID do evento"
// @Success      200 {object} evento.EventoResponseDto
// @Failure      404 {object} rest_err.RestErr
// @Router       /api/domain/eventos/eventos/{uuid} [get]
func (ctrl *controllerImpl) Read(c *gin.Context) {
	id, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError("UUID inválido.").ComCausa(err))
		return
	}

	e, err := ctrl.service.Read(c.Request.Context(), id)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, ParaResponseCompleto(*e))
}

// Resumo devolve a contagem por tipo do recorte pedido.
//
// @Summary      Resumo dos eventos
// @Tags         Valheim · Eventos
// @Produce      json
// @Param        horas query int false "Tamanho da janela em horas (0 = tudo). Padrão: 24"
// @Success      200 {object} evento.ResumoResponseDto
// @Router       /api/domain/eventos/resumo [get]
func (ctrl *controllerImpl) Resumo(c *gin.Context) {
	desde, err := janela(c.Query("horas"))
	if err != nil {
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Parâmetro `horas` inválido.").ComCausa(err))
		return
	}

	resumo, err := ctrl.service.Resumir(c.Request.Context(), desde)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, ParaResumoResponse(resumo))
}

// janela traduz `?horas=` no instante inicial do recorte.
//
// Ausente = 24h; `0` = desde sempre (é a forma de pedir o total histórico sem
// inventar um segundo parâmetro).
func janela(bruto string) (*time.Time, error) {
	horas := int64(JanelaPadraoDoResumo / time.Hour)
	if bruto != "" {
		lido, err := strconv.ParseInt(bruto, 10, 64)
		if err != nil || lido < 0 {
			return nil, ErrDataInvalida
		}
		horas = lido
	}
	if horas == 0 {
		return nil, nil
	}
	desde := time.Now().UTC().Add(-time.Duration(horas) * time.Hour)
	return &desde, nil
}

// responderErro traduz a sentinela do service no status HTTP.
//
// É o único lugar do subdomínio que decide status: o service fala em
// sentinelas, o controller fala em HTTP.
func (ctrl *controllerImpl) responderErro(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		rest_err.WriteError(c, obs, rest_err.NewNotFoundError("Evento não encontrado.").ComCausa(err))
	case errors.Is(err, ErrTipoInvalido):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Tipo de evento inválido.").ComCausa(err))
	case errors.Is(err, ErrDataInvalida):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Data inválida: use o formato RFC3339 (2026-08-09T00:00:00Z).").ComCausa(err))
	case errors.Is(err, ErrLinhaVazia):
		rest_err.WriteError(c, obs, rest_err.NewUnprocessableEntityError(
			"Linha de log vazia.").ComCausa(err))
	case errors.Is(err, ErrLinhaLonga):
		rest_err.WriteError(c, obs, rest_err.NewRequestEntityTooLargeError(
			"Linha de log acima do tamanho máximo.").ComCausa(err))
	default:
		rest_err.WriteError(c, obs, err)
	}
}
