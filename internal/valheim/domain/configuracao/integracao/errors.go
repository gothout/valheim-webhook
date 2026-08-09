package integracao

import "errors"

var (
	// ErrModoInvalido é o modo fora do vocabulário (`bot` | `webhook`).
	ErrModoInvalido = errors.New("modo de integração inválido")
	// ErrSemDestino é o pedido de LIGAR a integração sem preencher o destino
	// do modo escolhido.
	ErrSemDestino = errors.New("integração habilitada sem destino configurado")
	// ErrURLInvalida é o webhook que não é uma URL do Discord.
	ErrURLInvalida = errors.New("url de webhook inválida")
	// ErrEventoInvalido é o tipo de evento fora do vocabulário.
	ErrEventoInvalido = errors.New("tipo de evento inválido")
	// ErrNaoConfigurada é o teste pedido com a integração desligada.
	ErrNaoConfigurada = errors.New("integração não configurada")
	// ErrTeste é a falha da mensagem de teste.
	ErrTeste = errors.New("falha ao enviar a mensagem de teste")
	// ErrPersistencia embrulha a falha do banco.
	ErrPersistencia = errors.New("falha ao acessar a configuração")
)

// errCodes registra sentinela → código estável para o observador de erros.
var errCodes = map[error]string{
	ErrModoInvalido:   "ErrModoInvalido",
	ErrSemDestino:     "ErrSemDestino",
	ErrURLInvalida:    "ErrURLInvalida",
	ErrEventoInvalido: "ErrEventoInvalido",
	ErrNaoConfigurada: "ErrNaoConfigurada",
	ErrTeste:          "ErrTeste",
	ErrPersistencia:   "ErrPersistencia",
}
