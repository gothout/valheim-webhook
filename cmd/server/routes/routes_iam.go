package routes

import (
	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/iam/application/identidade/auth"
	"valheim-webhook/internal/iam/domain/identidade/usuario"
)

// RegisterIamRoutes registra as rotas do sistema transversal `iam`.
//
// Cada controller registra o próprio prefixo e os próprios middlewares — este
// arquivo só decide QUAL controller entra em QUAL camada. A ordem aqui é
// irrelevante: o que importa é a ordem de inicialização, em
// `cmd/bootstrap/domain_init.go`.
func RegisterIamRoutes(domain, application *gin.RouterGroup) {
	registrarRotas(domain, "iam/identidade/usuario", usuario.Use)

	// A sessão é caso de USO: costura a conta (domínio) com o emissor de token
	// (infra), e nenhum dos dois conhece o outro.
	registrarRotas(application, "iam/identidade/auth", auth.Use)
}
