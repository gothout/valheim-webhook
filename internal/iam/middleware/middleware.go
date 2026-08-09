// Package middleware é a cadeia de autorização do painel: quem é você, e você
// pode fazer isto?
//
// Ele NÃO importa `iam/domain` — os controllers de lá importam ELE, e o
// caminho contrário fecharia um ciclo. Tudo o que precisa do domínio entra por
// interface (`contratos.go`), ligada no `cmd/bootstrap`.
//
// Princípio que vale para todo o arquivo: **cadeia não inicializada = rota
// fechada**. Um erro de montagem no boot precisa produzir 403 em tudo, nunca
// uma API aberta que ninguém percebeu.
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/pkg/reqctx"
	"valheim-webhook/internal/pkg/rest_err"
)

// CookieSessao é o nome do cookie do painel.
const CookieSessao = "vw_sessao"

// PrefixoBearer é o esquema aceito no header Authorization.
const PrefixoBearer = "Bearer "

// Middleware é a cadeia de autorização.
type Middleware interface {
	// Autenticar exige sessão válida e põe a identidade no contexto.
	Autenticar() gin.HandlerFunc
	// ExigirAdministrador roda DEPOIS de Autenticar e barra quem não é admin.
	ExigirAdministrador() gin.HandlerFunc
}

type middlewareImpl struct {
	sessao     Sessao
	conferente Conferente
}

// NewMiddleware monta a cadeia com as dependências ligadas pelo boot.
func NewMiddleware(deps Dependencias) Middleware {
	return &middlewareImpl{sessao: deps.Sessao, conferente: deps.Conferente}
}

// Autenticar resolve a sessão.
//
// Ordem: token do cookie (o painel) ou do header (script e `curl`) → validação
// da assinatura → conferência da conta no banco → identidade no contexto.
func (m *middlewareImpl) Autenticar() gin.HandlerFunc {
	return func(c *gin.Context) {
		if m == nil || m.sessao == nil || m.conferente == nil {
			negar(c, http.StatusForbidden, "Autorização indisponível.", ErrNotInitialized)
			return
		}

		bruto := tokenDoRequest(c)
		if bruto == "" {
			negar(c, http.StatusUnauthorized, "Faça login para continuar.", ErrSessaoAusente)
			return
		}

		autor, err := m.sessao.Validar(bruto)
		if err != nil {
			switch {
			case errors.Is(err, ErrSessaoExpirada):
				negar(c, http.StatusUnauthorized, "Sessão expirada: faça login de novo.", err)
			default:
				negar(c, http.StatusUnauthorized, "Sessão inválida.", ErrSessaoInvalida)
			}
			return
		}

		// O token afirma o papel; o banco CONFIRMA. Entre um e outro pode ter
		// havido uma remoção, uma desativação ou um rebaixamento — e é o banco
		// que tem a resposta atual.
		papelAtual, ativo, err := m.conferente.Situacao(c.Request.Context(), autor.UUID)
		switch {
		case errors.Is(err, ErrUsuarioDesconhecido):
			negar(c, http.StatusUnauthorized, "Sessão inválida.", err)
			return
		case err != nil:
			negar(c, http.StatusServiceUnavailable, "Não foi possível confirmar a sessão agora.", err)
			return
		case !ativo:
			negar(c, http.StatusForbidden, "Conta inativa.", ErrContaInativa)
			return
		}
		autor.Papel = papelAtual

		c.Request = c.Request.WithContext(reqctx.ComIdentidade(c.Request.Context(), reqctx.Identidade{
			UsuarioUUID: autor.UUID,
			Email:       autor.Email,
			Nome:        autor.Nome,
			Papel:       string(autor.Papel),
		}))
		c.Next()
	}
}

// ExigirAdministrador barra quem não administra.
//
// Ele confia na identidade do contexto — que só existe se `Autenticar` rodou
// antes. Sem ela, o resultado é 403, e não "passa": rota que esqueceu o
// `Autenticar()` fica fechada em vez de ficar aberta.
func (m *middlewareImpl) ExigirAdministrador() gin.HandlerFunc {
	return func(c *gin.Context) {
		identidade := reqctx.Do(c.Request.Context())
		if !identidade.Autenticado() {
			negar(c, http.StatusForbidden, "Autorização indisponível.", ErrNotInitialized)
			return
		}
		if identidade.Papel != string(papelAdministrador) {
			negar(c, http.StatusForbidden,
				"Esta ação é restrita a administradores.", ErrSemPermissao)
			return
		}
		c.Next()
	}
}

// tokenDoRequest procura o token no cookie e, depois, no header.
func tokenDoRequest(c *gin.Context) string {
	if cookie, err := c.Cookie(CookieSessao); err == nil && strings.TrimSpace(cookie) != "" {
		return strings.TrimSpace(cookie)
	}
	cabecalho := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(cabecalho, PrefixoBearer) {
		return strings.TrimSpace(strings.TrimPrefix(cabecalho, PrefixoBearer))
	}
	return ""
}

// negar responde no formato padrão de erro e aborta a cadeia.
func negar(c *gin.Context, status int, mensagem string, causa error) {
	rest_err.WriteError(c, obs, rest_err.New(status, rotuloDe(status), mensagem).ComCausa(causa))
}

// rotuloDe traduz o status no rótulo estável do corpo de erro.
func rotuloDe(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return rest_err.ErroUnauthorized
	case http.StatusServiceUnavailable:
		return rest_err.ErroIndisponivel
	default:
		return "forbidden"
	}
}
