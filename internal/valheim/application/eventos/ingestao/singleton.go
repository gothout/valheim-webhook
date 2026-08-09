package ingestao

import (
	"errors"
	"log/slog"
	"sync"

	"valheim-webhook/internal/pkg/errobserve"
)

// Identidade do subdomínio (observador de erros e logs de boot).
const (
	Sistema    = "valheim"
	Dominio    = "eventos"
	Subdominio = "ingestao"
)

var (
	controllerInstance Controller
	serviceInstance    Service
	obs                *errobserve.Observer
	once               sync.Once
	initErr            error

	// ErrNotInitialized é o erro de quem pede o controller antes do boot.
	ErrNotInitialized = errors.New("ingestao controller not initialized")
)

// UseIngestao agrupa as camadas do subdomínio de aplicação (sem repository: ele
// não persiste nada próprio — quem grava são os subdomínios de domínio).
type UseIngestao struct {
	Service    Service
	Controller Controller
}

// New inicializa o singleton do subdomínio.
func New(deps Dependencias, opcoes Opcoes) (Controller, error) {
	once.Do(func() {
		if deps.Eventos == nil || deps.Jogadores == nil || deps.Politica == nil {
			initErr = errors.New("ingestao exige eventos, jogadores e política de notificação")
			return
		}
		serviceInstance = NewService(deps)
		controllerInstance = NewController(serviceInstance, opcoes)
		obs = errobserve.For(Sistema, Dominio, Subdominio, errCodes)

		// O aviso sai UMA vez, no boot: rota de ingestão aberta é decisão de
		// quem montou o processo, não evento de runtime. Em `app.env=prod` a
		// configuração nem passa pela validação sem token.
		if opcoes.Token == "" {
			slog.Warn("[BOOTSTRAP-DI] POST " + RotaEventos + " está ABERTO (sem ingestao.token): " +
				"aceitável apenas em rede privada — qualquer um que alcance esta porta " +
				"consegue inventar eventos no seu painel e no seu Discord")
		}
		if deps.Hub == nil {
			slog.Warn("[BOOTSTRAP-DI] ingestão sem barramento de tempo real: " +
				"o painel só atualiza ao recarregar")
		}
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
func MustUse() *UseIngestao {
	if controllerInstance == nil || serviceInstance == nil {
		panic(ErrNotInitialized)
	}
	return &UseIngestao{Service: serviceInstance, Controller: controllerInstance}
}
