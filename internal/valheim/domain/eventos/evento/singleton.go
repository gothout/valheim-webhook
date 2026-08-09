package evento

import (
	"errors"
	"sync"

	"gorm.io/gorm"

	"valheim-webhook/internal/pkg/errobserve"
)

var (
	controllerInstance Controller
	serviceInstance    Service
	repositoryInstance Repository
	obs                *errobserve.Observer
	once               sync.Once
	initErr            error

	// ErrNotInitialized é o erro de quem pede o controller antes do boot.
	ErrNotInitialized = errors.New("evento controller not initialized")
)

// UseEvento agrupa todas as camadas do subdomínio.
type UseEvento struct {
	Repository Repository
	Service    Service
	Controller Controller
}

// New inicializa o singleton do subdomínio.
func New(db *gorm.DB) (Controller, error) {
	once.Do(func() {
		if db == nil {
			initErr = errors.New("database connection cannot be nil")
			return
		}
		repositoryInstance = NewRepository(db)
		serviceInstance = NewService(repositoryInstance)
		controllerInstance = NewController(serviceInstance)
		// Observador de erros do subdomínio — sempre por último.
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

// MustUse devolve todas as camadas; entra em pânico se não inicializado.
func MustUse() *UseEvento {
	if controllerInstance == nil || serviceInstance == nil || repositoryInstance == nil {
		panic(ErrNotInitialized)
	}
	return &UseEvento{
		Repository: repositoryInstance,
		Service:    serviceInstance,
		Controller: controllerInstance,
	}
}
