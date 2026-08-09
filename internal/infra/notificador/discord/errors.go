package discord

import "errors"

var (
	// ErrEnvio embrulha qualquer recusa da API do Discord.
	ErrEnvio = errors.New("falha ao publicar no discord")
	// ErrCredencial é o 401/403: token errado, ou o app sem permissão de
	// escrever no canal. Não adianta repetir.
	ErrCredencial = errors.New("credencial do discord recusada")
	// ErrLimiteExcedido é o 429 cuja espera pedida passa do teto: repetir só
	// atrasaria os eventos seguintes da fila.
	ErrLimiteExcedido = errors.New("limite de taxa do discord acima do teto de espera")
)
