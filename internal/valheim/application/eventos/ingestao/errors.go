package ingestao

import "errors"

var (
	// ErrCorpoVazio é o `POST /eventos` sem linha nenhuma.
	ErrCorpoVazio = errors.New("corpo sem linha de log")
	// ErrCorpoInvalido é o JSON malformado.
	ErrCorpoInvalido = errors.New("corpo inválido")
	// ErrCorpoGrande é o corpo acima do teto de `ingestao.max_body_bytes`.
	ErrCorpoGrande = errors.New("corpo acima do tamanho máximo")
	// ErrTokenInvalido é o `X-Ingest-Token` ausente ou errado.
	ErrTokenInvalido = errors.New("token de ingestão inválido")
	// ErrLoteGrande é o lote com mais linhas do que o teto.
	ErrLoteGrande = errors.New("lote com linhas demais")
	// ErrRegistro embrulha a falha ao gravar (o único erro que faz o hook do
	// container ver um 5xx e, com `curl -f`, reclamar no log do servidor).
	ErrRegistro = errors.New("falha ao registrar o evento")
	// ErrPresencaDesconhecida é a saída de um objeto que não pertence a nenhum
	// personagem online. Não é falha: acontece toda vez que o mundo reinicia e
	// o servidor limpa o que ficou da sessão anterior.
	ErrPresencaDesconhecida = errors.New("presença desconhecida")
	// ErrEventoIgnorado é a linha que chegou, foi entendida e NÃO virou evento
	// novo — hoje, as repetições da mesma saída (o Valheim emite uma linha por
	// objeto abandonado). Vira contador na resposta, nunca erro para quem
	// chamou: a linha não se perdeu, ela é a mesma notícia de novo.
	ErrEventoIgnorado = errors.New("linha ignorada: repetição de um fato já registrado")
)

// errCodes registra sentinela → código estável para o observador de erros.
var errCodes = map[error]string{
	ErrCorpoVazio:           "ErrCorpoVazio",
	ErrCorpoInvalido:        "ErrCorpoInvalido",
	ErrCorpoGrande:          "ErrCorpoGrande",
	ErrTokenInvalido:        "ErrTokenInvalido",
	ErrLoteGrande:           "ErrLoteGrande",
	ErrRegistro:             "ErrRegistro",
	ErrPresencaDesconhecida: "ErrPresencaDesconhecida",
	ErrEventoIgnorado:       "ErrEventoIgnorado",
}
