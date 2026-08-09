package jwt

import (
	"log/slog"
	"sync"

	"valheim-webhook/internal/pkg/config"
)

var (
	instancia *Sessao
	once      sync.Once
	initErr   error
)

// Init monta o emissor do processo uma única vez. Falha aqui é fatal.
func Init(cfg config.Security) (*Sessao, error) {
	once.Do(func() {
		sessao, err := Novo(cfg)
		if err != nil {
			initErr = err
			return
		}
		instancia = sessao

		if sessao.Sorteado() {
			slog.Warn("[JWT] segredo de sessão SORTEADO neste boot: " +
				"preencha security.jwt_secret (ou VALHEIM_WEBHOOK_JWT_SECRET) " +
				"para as sessões sobreviverem a um reinício")
		}
	})
	return instancia, initErr
}

// Use devolve o emissor do processo; erro se Init ainda não rodou.
func Use() (*Sessao, error) {
	if instancia == nil {
		return nil, ErrNotInitialized
	}
	return instancia, nil
}

// MustUse devolve o emissor e entra em pânico se não houver. Uso restrito ao
// boot.
func MustUse() *Sessao {
	if instancia == nil {
		panic(ErrNotInitialized)
	}
	return instancia
}
