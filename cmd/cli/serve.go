package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"valheim-webhook/cmd/bootstrap"
)

// newServeCommand cria o comando que sobe o serviço.
func newServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Sobe o receptor de eventos, a API e o painel",
		Long: "Sobe o serviço: carrega a configuração, conecta no Postgres, aplica as " +
			"migrations pendentes (quando migrations.auto_run=true), registra os domínios, " +
			"garante a conta administrativa inicial e serve o painel, a API e o " +
			"POST /eventos.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// SIGINT (Ctrl+C) e SIGTERM (`docker stop`) cancelam o contexto; o
			// servidor então drena os requests em voo antes de encerrar.
			ctx, parar := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer parar()

			return bootstrap.Start(ctx, ConfigPath(cmd))
		},
	}
}
