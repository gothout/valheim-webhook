package trilhas

import (
	"errors"
	"sync"

	"valheim-webhook/internal/pkg/errobserve"
)

var (
	controllerInstance Controller
	serviceInstance    Service
	obs                *errobserve.Observer
	once               sync.Once
	initErr            error

	// ErrNotInitialized é o erro de quem pede o controller antes do boot.
	ErrNotInitialized = errors.New("trilhas controller not initialized")
)

// UseTrilhas agrupa as camadas do subdomínio (sem repository: a fonte é o anel
// em memória).
type UseTrilhas struct {
	Service    Service
	Controller Controller
}

// New inicializa o singleton do subdomínio.
//
// Fonte nula é ACEITA: o subdomínio sobe e responde 503 na rota, em vez de
// derrubar o boot. Um painel de logs indisponível é um incômodo; um receptor de
// eventos que não sobe por causa dele seria um defeito.
func New(deps Dependencias) (Controller, error) {
	once.Do(func() {
		serviceInstance = NewService(deps)
		controllerInstance = NewController(serviceInstance)
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
func MustUse() *UseTrilhas {
	if controllerInstance == nil || serviceInstance == nil {
		panic(ErrNotInitialized)
	}
	return &UseTrilhas{Service: serviceInstance, Controller: controllerInstance}
}
