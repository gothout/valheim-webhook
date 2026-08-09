package auth

import (
	"errors"
	"sync"

	"valheim-webhook/internal/pkg/errobserve"
)

// Identidade do subdomínio (observador de erros e logs de boot).
const (
	Sistema    = "iam"
	Dominio    = "identidade"
	Subdominio = "auth"
)

var (
	controllerInstance Controller
	serviceInstance    Service
	obs                *errobserve.Observer
	once               sync.Once
	initErr            error

	// ErrNotInitialized é o erro de quem pede o controller antes do boot.
	ErrNotInitialized = errors.New("auth controller not initialized")
)

// UseAuth agrupa as camadas do subdomínio de aplicação (sem repository: ele não
// persiste nada próprio).
type UseAuth struct {
	Service    Service
	Controller Controller
}

// New inicializa o singleton do subdomínio.
func New(deps Dependencias, opcoes Opcoes) (Controller, error) {
	once.Do(func() {
		if deps.Usuarios == nil || deps.Emissor == nil {
			initErr = errors.New("auth exige o serviço de usuários e o emissor de sessão")
			return
		}
		serviceInstance = NewService(deps)
		controllerInstance = NewController(serviceInstance, opcoes)
		obs = errobserve.For(Sistema, Dominio, Subdominio, errCodes)
	})
	return controllerInstance, initErr
}

// Use devolve o controller singleton; erro se não inicializado.
func Use() (Controller, error) {
	if controllerInstance == nil {
		return nil, ErrNotInitialized
	}
	return controllerInstance, nil
}

// MustUse devolve as camadas; entra em pânico se não inicializado.
func MustUse() *UseAuth {
	if controllerInstance == nil || serviceInstance == nil {
		panic(ErrNotInitialized)
	}
	return &UseAuth{Service: serviceInstance, Controller: controllerInstance}
}
