package postgres

import "errors"

var (
	// ErrNotInitialized é o erro de quem pede a conexão antes do boot.
	ErrNotInitialized = errors.New("conexão com o postgres não inicializada")
	// ErrConexao embrulha qualquer falha de abertura do pool.
	ErrConexao = errors.New("falha ao conectar no postgres")
)
