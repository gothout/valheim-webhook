package memoria

import (
	"log/slog"
	"os"
	"sync"
)

var (
	instancia *Buffer
	once      sync.Once
)

// Init instala o anel como espelho do log padrão do processo.
//
// Chamado na PRIMEIRA linha do boot, antes de qualquer coisa que possa logar:
// o valor de um painel de logs está justamente em conter o boot, que é onde
// aparecem "conectado", "[DEGRADADO]" e "migrations aplicadas".
//
// O destino de baixo continua sendo o stdout em texto — legível no `docker
// logs` sem ferramenta nenhuma.
func Init(capacidade int, nivel slog.Level) *Buffer {
	once.Do(func() {
		instancia = NovoBuffer(capacidade)
		base := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: nivel})
		slog.SetDefault(slog.New(NovoHandler(base, instancia)))
	})
	return instancia
}

// Use devolve o anel do processo — nulo se Init não rodou (o painel de logs
// responde vazio, e nada quebra).
func Use() *Buffer { return instancia }
