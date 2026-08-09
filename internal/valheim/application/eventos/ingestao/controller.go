package ingestao

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/iam/middleware"
	"valheim-webhook/internal/pkg/rest_err"
	"valheim-webhook/internal/pkg/tempo_real"
)

// RotaEventos é a porta de entrada, na RAIZ do servidor.
//
// Ela não vive sob `/api/application/...` como as outras rotas deste caso de
// uso, e isso é deliberado: o endereço é escrito à mão dentro de uma variável
// de ambiente do container de Valheim
// (`http://host.docker.internal:6060/eventos`). Um caminho curto e estável ali
// vale mais do que a simetria da árvore de rotas — e mudá-lo depois exigiria
// recriar o container do servidor de jogo.
const RotaEventos = "/eventos"

// HeaderToken é o segredo compartilhado com o hook do container.
const HeaderToken = "X-Ingest-Token"

// TipoTexto é o Content-Type do caminho RECOMENDADO de ingestão.
const TipoTexto = "text/plain"

// Controller é a borda HTTP do caso de uso.
type Controller interface {
	// Routes registra as rotas de consulta e o fluxo SSE (sob `/api`).
	Routes(routes gin.IRouter)
	// RotasRaiz registra o `POST /eventos` na raiz do servidor.
	RotasRaiz(routes gin.IRouter)
	Receber(c *gin.Context)
	Stream(c *gin.Context)
}

type controllerImpl struct {
	service Service
	opcoes  Opcoes
}

// Opcoes são as decisões de borda da ingestão.
type Opcoes struct {
	// Token exigido no header. Vazio deixa a rota aberta (ver o aviso do boot).
	Token string
	// MaxBodyBytes é o teto do corpo aceito.
	MaxBodyBytes int64
}

// NewController monta o controller.
func NewController(service Service, opcoes Opcoes) Controller {
	return &controllerImpl{service: service, opcoes: opcoes}
}

// Routes registra o que fica sob `/api/application`.
func (ctrl *controllerImpl) Routes(routes gin.IRouter) {
	mw := middleware.MustUse().Middleware
	g := routes.Group("/eventos/ingestao")

	// O fluxo ao vivo exige sessão como qualquer outra leitura do painel. O
	// navegador manda o cookie sozinho no EventSource — não é preciso (nem
	// possível) pôr header nele.
	g.GET("/stream", mw.Autenticar(), ctrl.Stream)
}

// RotasRaiz registra a porta de entrada dos eventos.
//
// Ela NÃO passa pela sessão do painel: quem chama é um `curl` dentro do
// container do servidor de jogo, que não faz login. A proteção é o token
// compartilhado do `X-Ingest-Token`.
func (ctrl *controllerImpl) RotasRaiz(routes gin.IRouter) {
	routes.POST(RotaEventos, ctrl.Receber)
}

// Receber é o `POST /eventos`.
//
// Aceita três formatos de corpo, e a ordem de preferência é ao contrário da
// ordem histórica:
//
//  1. `text/plain` com a linha CRUA (recomendado). É o único que não tem como
//     quebrar: linha de log do Valheim contém aspas (`Got text "..."`), e
//     montar JSON com `-d "{\"log\":\"$l\"}"` no shell produz JSON inválido
//     assim que a primeira aspa aparece;
//  2. `{"log": "..."}` — o formato do exemplo mais divulgado;
//  3. `{"logs": ["...", "..."]}` — para quem acumula antes de mandar.
//
// Responde 202 (aceito) porque o efeito completo — mensagem no Discord — é
// assíncrono. O que o 202 garante é que o evento foi GRAVADO.
//
// @Summary  Recebe uma linha de log do servidor de Valheim
// @Tags     Valheim · Ingestão
// @Accept   json,plain
// @Produce  json
// @Param    X-Ingest-Token header string false "Token compartilhado, quando configurado"
// @Success  202 {object} ingestao.IngestaoResponseDto
// @Failure  401 {object} rest_err.RestErr
// @Failure  413 {object} rest_err.RestErr
// @Router   /eventos [post]
func (ctrl *controllerImpl) Receber(c *gin.Context) {
	if !ctrl.tokenConfere(c) {
		rest_err.WriteError(c, obs, rest_err.NewUnauthorizedError(
			"Token de ingestão ausente ou inválido.").ComCausa(ErrTokenInvalido))
		return
	}

	linhas, err := ctrl.lerCorpo(c)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	resultados, ignorados, err := ctrl.service.RegistrarLote(c.Request.Context(), linhas)
	if err != nil {
		ctrl.responderErro(c, err)
		return
	}

	eventos := make([]EventoAceitoDto, 0, len(resultados))
	for _, r := range resultados {
		eventos = append(eventos, EventoAceitoDto{
			UUID:       r.Registro.UUID,
			Tipo:       r.Registro.Tipo,
			Jogador:    primeiroNaoVazio(r.Registro.Jogador, r.Presenca.Nome),
			OcorridoEm: r.Registro.OcorridoEm,
			Notificado: r.Notificado,
		})
	}

	c.JSON(http.StatusAccepted, IngestaoResponseDto{
		Recebidos: len(eventos),
		Ignorados: ignorados,
		Eventos:   eventos,
	})
}

// Stream é o fluxo SSE do painel.
//
// @Summary  Fluxo ao vivo dos eventos (SSE)
// @Tags     Valheim · Ingestão
// @Produce  text/event-stream
// @Success  200 {string} string "fluxo de eventos"
// @Router   /api/application/eventos/ingestao/stream [get]
func (ctrl *controllerImpl) Stream(c *gin.Context) {
	hub := ctrl.service.Hub()
	if hub == nil {
		rest_err.WriteError(c, obs, rest_err.NewServiceUnavailableError(
			"O fluxo ao vivo está desligado neste processo."))
		return
	}
	tempo_real.Handler(hub)(c)
}

// lerCorpo extrai as linhas de log do corpo, seja qual for o formato.
func (ctrl *controllerImpl) lerCorpo(c *gin.Context) ([]string, error) {
	limite := ctrl.opcoes.MaxBodyBytes
	if limite <= 0 {
		limite = 64 << 10
	}

	// O +1 é o que permite DISTINGUIR "corpo do tamanho exato do teto" de
	// "corpo maior que o teto": sem ele, um corpo grande demais chegaria
	// truncado e viraria uma linha de log cortada pela metade, gravada como se
	// fosse o que o servidor disse.
	bruto, err := io.ReadAll(io.LimitReader(c.Request.Body, limite+1))
	if err != nil {
		return nil, ErrCorpoInvalido
	}
	if int64(len(bruto)) > limite {
		return nil, ErrCorpoGrande
	}
	if len(strings.TrimSpace(string(bruto))) == 0 {
		return nil, ErrCorpoVazio
	}

	tipo := strings.ToLower(strings.TrimSpace(c.ContentType()))
	if strings.HasPrefix(tipo, "application/json") {
		var req IngestaoRequestDto
		if err := json.Unmarshal(bruto, &req); err != nil {
			return nil, ErrCorpoInvalido
		}
		linhas := req.Linhas()
		if len(linhas) == 0 {
			return nil, ErrCorpoVazio
		}
		return linhas, nil
	}

	// Qualquer outro Content-Type (inclusive nenhum) é tratado como texto: o
	// `curl --data-binary @-` do hook não manda tipo nenhum, e recusar por
	// causa disso transformaria um detalhe de configuração num evento perdido.
	return strings.Split(strings.ReplaceAll(string(bruto), "\r\n", "\n"), "\n"), nil
}

// tokenConfere valida o segredo compartilhado.
//
// Comparação em tempo constante: comparar segredo com `==` vaza, pelo tempo, o
// tamanho do prefixo que já está certo.
func (ctrl *controllerImpl) tokenConfere(c *gin.Context) bool {
	esperado := strings.TrimSpace(ctrl.opcoes.Token)
	if esperado == "" {
		return true // rota aberta: o boot já avisou
	}
	recebido := strings.TrimSpace(c.GetHeader(HeaderToken))
	return subtle.ConstantTimeCompare([]byte(esperado), []byte(recebido)) == 1
}

// responderErro traduz a sentinela do service no status HTTP.
func (ctrl *controllerImpl) responderErro(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrCorpoVazio):
		rest_err.WriteError(c, obs, rest_err.NewUnprocessableEntityError(
			"Corpo sem linha de log.").ComCausa(err))
	case errors.Is(err, ErrCorpoInvalido):
		rest_err.WriteError(c, obs, rest_err.NewBadRequestError(
			"Corpo inválido. Mande a linha crua como text/plain, ou {\"log\": \"...\"} como JSON.").ComCausa(err))
	case errors.Is(err, ErrCorpoGrande):
		rest_err.WriteError(c, obs, rest_err.NewRequestEntityTooLargeError(
			"Corpo acima do tamanho máximo.").ComCausa(err))
	case errors.Is(err, ErrLoteGrande):
		rest_err.WriteError(c, obs, rest_err.NewRequestEntityTooLargeError(
			"Lote com linhas demais: mande em partes menores.").ComCausa(err))
	default:
		rest_err.WriteError(c, obs, err)
	}
}
