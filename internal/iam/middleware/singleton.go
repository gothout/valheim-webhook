package middleware

import (
	"sync"

	"valheim-webhook/internal/pkg/errobserve"
	"valheim-webhook/internal/pkg/papel"
)

// papelAdministrador é o papel que abre as rotas de administração.
const papelAdministrador = papel.Admin

var (
	instancia *UseMiddleware
	obs       *errobserve.Observer
	once      sync.Once
)

// UseMiddleware é o que os controllers recebem do MustUse.
type UseMiddleware struct {
	Middleware Middleware
}

// New inicializa a cadeia do processo.
//
// Roda ANTES dos subdomínios no boot: os controllers deles chamam `MustUse()`
// dentro do `Routes()`, e o registro de rotas acontece depois.
func New(deps Dependencias) (Middleware, error) {
	once.Do(func() {
		obs = errobserve.For("iam", "middleware", "autorizacao", errCodes)
		instancia = &UseMiddleware{Middleware: NewMiddleware(deps)}
	})
	if instancia == nil {
		return nil, ErrNotInitialized
	}
	return instancia.Middleware, nil
}

// Use devolve a cadeia; erro se não inicializada.
func Use() (Middleware, error) {
	if instancia == nil {
		return nil, ErrNotInitialized
	}
	return instancia.Middleware, nil
}

// MustUse devolve a cadeia e entra em pânico se não houver.
//
// É chamado pelos controllers dentro do `Routes()`. O pânico é o comportamento
// certo aqui: registrar rota sem cadeia de autorização produziria uma API
// ABERTA, e um processo que não sobe é melhor do que um que sobe assim.
func MustUse() *UseMiddleware {
	if instancia == nil {
		panic(ErrNotInitialized)
	}
	return instancia
}
