package evento

import "errors"

var (
	// ErrLinhaVazia é o corpo que chegou sem linha de log nenhuma.
	ErrLinhaVazia = errors.New("linha de log vazia")
	// ErrLinhaLonga é a linha acima do teto de tamanho: log de jogo não tem
	// linha de 8 KB, e aceitar uma seria aceitar qualquer coisa.
	ErrLinhaLonga = errors.New("linha de log acima do tamanho máximo")
	// ErrNotFound é o evento inexistente.
	ErrNotFound = errors.New("evento não encontrado")
	// ErrSaidaRepetida é uma das dezenas de linhas `Destroying abandoned…` que
	// o servidor emite na MESMA saída. Não é falha: é a segunda em diante do
	// mesmo fato, e registrá-la de novo encheria o feed com a mesma notícia.
	ErrSaidaRepetida = errors.New("saída já registrada nesta janela")
	// ErrTipoInvalido é o filtro pedindo um tipo fora do vocabulário.
	ErrTipoInvalido = errors.New("tipo de evento inválido")
	// ErrDataInvalida é o filtro com data fora do formato RFC3339.
	ErrDataInvalida = errors.New("data do filtro inválida (use RFC3339)")
	// ErrPersistencia embrulha a falha do banco — o único erro daqui que não é
	// culpa de quem chamou.
	ErrPersistencia = errors.New("falha ao acessar os eventos")
)

// errCodes registra sentinela → código estável para o observador de erros.
// Obrigatório: todo sentinela novo entra neste mapa.
var errCodes = map[error]string{
	ErrLinhaVazia:    "ErrLinhaVazia",
	ErrLinhaLonga:    "ErrLinhaLonga",
	ErrNotFound:      "ErrNotFound",
	ErrSaidaRepetida: "ErrSaidaRepetida",
	ErrTipoInvalido:  "ErrTipoInvalido",
	ErrDataInvalida:  "ErrDataInvalida",
	ErrPersistencia:  "ErrPersistencia",
}
