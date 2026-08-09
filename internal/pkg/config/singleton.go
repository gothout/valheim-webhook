package config

import (
	"errors"
	"sync"
)

var (
	instance *Config
	once     sync.Once
	initErr  error

	// ErrNotInitialized indica uso da configuração antes do boot chamar Init.
	ErrNotInitialized = errors.New("configuração não inicializada (chame config.Init no boot)")
)

// Init carrega a configuração uma única vez no processo e guarda o singleton.
//
// Chamado no início de cada comando da CLI, sempre com o caminho da flag
// `--config` (vazio = busca padrão). Erro aqui é fatal.
func Init(path string) (*Config, error) {
	once.Do(func() {
		cfg, err := Environment(path)
		if err != nil {
			initErr = err
			return
		}
		instance = cfg
	})
	return instance, initErr
}

// Use devolve a configuração carregada; erro se Init ainda não rodou.
func Use() (*Config, error) {
	if instance == nil {
		return nil, ErrNotInitialized
	}
	return instance, nil
}

// MustUse devolve a configuração carregada; entra em pânico se não houver.
// Uso restrito ao boot, onde a ausência de configuração é irrecuperável.
func MustUse() *Config {
	if instance == nil {
		panic(ErrNotInitialized)
	}
	return instance
}
