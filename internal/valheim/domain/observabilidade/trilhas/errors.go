package trilhas

import "errors"

var (
	// ErrFonteIndisponivel é o painel de logs pedido num processo em que o anel
	// não foi instalado (só acontece em teste ou em ferramenta).
	ErrFonteIndisponivel = errors.New("trilha de log indisponível neste processo")
	// ErrNivelInvalido é o filtro com um nível que não existe.
	ErrNivelInvalido = errors.New("nível de log inválido")
)

// errCodes registra sentinela → código estável para o observador de erros.
var errCodes = map[error]string{
	ErrFonteIndisponivel: "ErrFonteIndisponivel",
	ErrNivelInvalido:     "ErrNivelInvalido",
}
