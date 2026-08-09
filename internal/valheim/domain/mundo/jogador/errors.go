package jogador

import "errors"

var (
	// ErrNomeVazio é a tentativa de registrar acontecimento sem personagem.
	ErrNomeVazio = errors.New("nome do personagem vazio")
	// ErrNotFound é o personagem inexistente.
	ErrNotFound = errors.New("personagem não encontrado")
	// ErrPersistencia embrulha a falha do banco.
	ErrPersistencia = errors.New("falha ao acessar os personagens")
)

// errCodes registra sentinela → código estável para o observador de erros.
var errCodes = map[error]string{
	ErrNomeVazio:    "ErrNomeVazio",
	ErrNotFound:     "ErrNotFound",
	ErrPersistencia: "ErrPersistencia",
}
