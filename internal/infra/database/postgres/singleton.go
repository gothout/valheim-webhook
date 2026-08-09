package postgres

import (
	"sync"

	"gorm.io/gorm"

	"valheim-webhook/internal/pkg/config"
)

var (
	instancia *Conn
	once      sync.Once
	initErr   error
)

// InitPostgres abre a conexão do processo uma única vez e devolve o *gorm.DB.
//
// Chamada no boot, antes das migrations e do InitDomains. Falha aqui é fatal.
// Chamadas seguintes devolvem sempre a mesma conexão — ou o mesmo erro, se a
// primeira tentativa falhou.
func InitPostgres(cfg config.Postgres) (*gorm.DB, error) {
	once.Do(func() {
		conn, err := Connect(cfg)
		if err != nil {
			initErr = err
			return
		}
		instancia = conn
	})
	if initErr != nil {
		return nil, initErr
	}
	return instancia.DB(), nil
}

// GetDB devolve o *gorm.DB do processo; erro se InitPostgres ainda não rodou.
func GetDB() (*gorm.DB, error) {
	if instancia == nil {
		return nil, ErrNotInitialized
	}
	return instancia.DB(), nil
}

// MustGetDB devolve o *gorm.DB do processo e entra em pânico se não houver.
// Uso restrito ao boot, onde a ausência é irrecuperável.
func MustGetDB() *gorm.DB {
	if instancia == nil {
		panic(ErrNotInitialized)
	}
	return instancia.DB()
}

// Use devolve a conexão completa (gorm + *sql.DB + encerramento).
func Use() (*Conn, error) {
	if instancia == nil {
		return nil, ErrNotInitialized
	}
	return instancia, nil
}

// Disponivel diz se o pool do processo está aberto (health check da API).
func Disponivel() bool { return instancia != nil && instancia.Saudavel() }

// Close encerra o health check e o pool do processo. Sem conexão aberta é
// no-op — o shutdown pode chamar sem saber se o boot chegou até aqui.
func Close() error {
	if instancia == nil {
		return nil
	}
	return instancia.Close()
}
