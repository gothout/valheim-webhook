// Package rest_err padroniza o corpo de erro de toda a API.
//
// Formato único, para qualquer rota e qualquer subdomínio:
//
//	{"code": 422, "error": "unprocessable_entity", "message": "Linha de log vazia.", "ray_trace": "5c2e..."}
//
// O fluxo esperado no controller é: o service devolve um erro sentinela, o
// controller traduz para o RestErr do status certo e responde com WriteError,
// que preenche o `ray_trace`, registra o evento no observador de erros
// (errobserve) e aborta a cadeia do Gin.
//
// Pacote folha de `internal/pkg`: importa reqctx (irmão), gin e stdlib — nunca
// `domain`, `application` ou `infra`.
package rest_err

import (
	"errors"
	"net/http"
)

// Rótulos estáveis do campo `error` — fazem parte do contrato da API e são o
// que o cliente usa para decidir comportamento (nunca a mensagem, que muda).
const (
	ErroBadRequest    = "bad_request"
	ErroUnauthorized  = "unauthorized"
	ErroNotFound      = "not_found"
	ErroUnprocessable = "unprocessable_entity"
	ErroTooLarge      = "payload_too_large"
	ErroInternal      = "internal_server_error"
	ErroIndisponivel  = "service_unavailable"
)

// RestErr é o erro HTTP padronizado.
//
// Implementa `error`: pode ser devolvido, comparado com errors.Is/As e
// encadeado com a causa de domínio (ComCausa) sem perder o status.
type RestErr struct {
	Code     int    `json:"code" example:"422"`
	Err      string `json:"error" example:"unprocessable_entity"`
	Message  string `json:"message" example:"Linha de log vazia."`
	RayTrace string `json:"ray_trace,omitempty" example:"5c2e2f5a-1f0b-4a1c-9f0e-2b7b6e0a1a11"`

	// causa é o erro sentinela do subdomínio que originou a resposta. Não vai
	// para o JSON (mensagem interna não vaza para o cliente), mas é o que o
	// observador de erros usa para resolver o `err_code` do evento.
	causa error
}

// Error implementa a interface error.
func (r *RestErr) Error() string { return r.Message }

// Unwrap expõe a causa de domínio para errors.Is/As.
func (r *RestErr) Unwrap() error { return r.causa }

// ComCausa encadeia o erro sentinela que originou a resposta.
//
//	return rest_err.NewUnprocessableEntityError("Linha vazia.").ComCausa(evento.ErrLinhaVazia)
//
// É o que permite ao errobserve gravar `err_code=ErrLinhaVazia` em vez de
// `ErrUnknown`.
func (r *RestErr) ComCausa(err error) *RestErr {
	r.causa = err
	return r
}

// ComRayTrace fixa o identificador de correlação. Normalmente não é preciso
// chamar: WriteError puxa o ray_trace do contexto do request.
func (r *RestErr) ComRayTrace(rayTrace string) *RestErr {
	r.RayTrace = rayTrace
	return r
}

// Origem devolve a causa de domínio, ou o próprio RestErr quando não houver.
// Sempre devolve um erro não nulo — é o valor que vai para o observador.
func (r *RestErr) Origem() error {
	if r.causa != nil {
		return r.causa
	}
	return r
}

// New monta um RestErr com status arbitrário. Prefira os construtores nomeados.
func New(code int, err, message string) *RestErr {
	return &RestErr{Code: code, Err: err, Message: message}
}

// NewBadRequestError — 400: corpo/query inválido.
func NewBadRequestError(message string) *RestErr {
	return New(http.StatusBadRequest, ErroBadRequest, message)
}

// NewUnauthorizedError — 401: token de ingestão ausente ou errado.
func NewUnauthorizedError(message string) *RestErr {
	return New(http.StatusUnauthorized, ErroUnauthorized, message)
}

// NewNotFoundError — 404: recurso inexistente.
func NewNotFoundError(message string) *RestErr {
	return New(http.StatusNotFound, ErroNotFound, message)
}

// NewRequestEntityTooLargeError — 413: corpo acima do teto da ingestão.
func NewRequestEntityTooLargeError(message string) *RestErr {
	return New(http.StatusRequestEntityTooLarge, ErroTooLarge, message)
}

// NewUnprocessableEntityError — 422: corpo válido, conteúdo impossível de
// aproveitar (linha de log vazia, por exemplo).
func NewUnprocessableEntityError(message string) *RestErr {
	return New(http.StatusUnprocessableEntity, ErroUnprocessable, message)
}

// NewServiceUnavailableError — 503: dependência degradada (o painel pedindo
// dado que só existe com banco de pé).
func NewServiceUnavailableError(message string) *RestErr {
	return New(http.StatusServiceUnavailable, ErroIndisponivel, message)
}

// NewInternalServerError — 500: falha interna. A mensagem devolvida ao cliente
// nunca carrega detalhe de infraestrutura; o diagnóstico vai pelo `ray_trace`.
func NewInternalServerError(message string) *RestErr {
	return New(http.StatusInternalServerError, ErroInternal, message)
}

// MensagemInterna é a resposta padrão de 500 — genérica de propósito.
const MensagemInterna = "Erro interno ao processar a requisição."

// De converte um erro qualquer em RestErr.
//
// RestErr (ou erro que embrulhe um) volta como está; qualquer outro vira 500
// com mensagem genérica e a causa preservada para o observador. É a rede de
// segurança do controller: erro não mapeado no switch nunca vaza detalhe
// interno para o cliente.
func De(err error) *RestErr {
	if err == nil {
		return nil
	}
	var r *RestErr
	if errors.As(err, &r) {
		return r
	}
	return NewInternalServerError(MensagemInterna).ComCausa(err)
}
