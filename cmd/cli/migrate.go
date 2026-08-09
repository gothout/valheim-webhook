package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"valheim-webhook/internal/infra/database/migrations"
	"valheim-webhook/internal/infra/database/postgres"
	"valheim-webhook/internal/pkg/config"
)

// newMigrateCommand monta a superfície `migrate up|down N|goto V|force V|status|validate`.
func newMigrateCommand() *cobra.Command {
	migrate := &cobra.Command{
		Use:   "migrate",
		Short: "Aplica, reverte e inspeciona o schema do banco",
	}

	migrate.AddCommand(
		comandoSimples("up", "Aplica todas as migrations pendentes",
			func(r *migrations.Runner, _ []string) error { return r.Up() }),

		&cobra.Command{
			Use:   "down N",
			Short: "Desfaz as N últimas migrations",
			Args:  cobra.ExactArgs(1),
			RunE: comRunner(func(r *migrations.Runner, args []string) error {
				n, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("quantidade inválida: %q", args[0])
				}
				return r.Down(n)
			}),
		},

		&cobra.Command{
			Use:   "goto VERSAO",
			Short: "Leva o schema até a versão informada",
			Args:  cobra.ExactArgs(1),
			RunE: comRunner(func(r *migrations.Runner, args []string) error {
				versao, err := strconv.ParseUint(args[0], 10, 64)
				if err != nil {
					return fmt.Errorf("versão inválida: %q", args[0])
				}
				return r.Goto(uint(versao))
			}),
		},

		&cobra.Command{
			Use:   "force VERSAO",
			Short: "Marca a versão como aplicada SEM rodar nada (conserta schema sujo)",
			Long: "Use apenas depois de conferir, no banco, qual é o estado real das " +
				"tabelas. `force` não executa SQL nenhum: ele só reescreve a versão " +
				"registrada — é a saída de um schema que ficou sujo porque uma migration " +
				"falhou no meio.",
			Args: cobra.ExactArgs(1),
			RunE: comRunner(func(r *migrations.Runner, args []string) error {
				versao, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("versão inválida: %q", args[0])
				}
				return r.Force(versao)
			}),
		},

		&cobra.Command{
			Use:   "status",
			Short: "Mostra a versão aplicada e o que falta",
			Args:  cobra.NoArgs,
			RunE: comRunner(func(r *migrations.Runner, _ []string) error {
				status, err := r.Status()
				if err != nil {
					return err
				}
				imprimirStatus(status)
				return nil
			}),
		},

		&cobra.Command{
			Use:   "validate",
			Short: "Confere os arquivos de migration SEM abrir conexão",
			Long: "Confere numeração, sequência e a existência do par `.down.sql` de cada " +
				"migration. Não abre conexão nenhuma — roda com o banco fora do ar, e é o " +
				"que entra no CI.",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				cfg, err := config.Init(ConfigPath(cmd))
				if err != nil {
					return err
				}
				arquivos, err := migrations.Validar(cfg.Migrations.Path)
				if err != nil {
					return err
				}
				fmt.Printf("%d migrations válidas em %s\n", len(arquivos), cfg.Migrations.Path)
				return nil
			},
		},
	)

	return migrate
}

// comandoSimples monta um subcomando sem argumentos que só chama o runner.
func comandoSimples(uso, descricao string, acao func(*migrations.Runner, []string) error) *cobra.Command {
	return &cobra.Command{
		Use:   uso,
		Short: descricao,
		Args:  cobra.NoArgs,
		RunE:  comRunner(acao),
	}
}

// comRunner abre a conexão dedicada, monta o runner, roda a ação e fecha tudo.
//
// Cada comando de migration abre e fecha a própria conexão: migration é
// operação pontual, e deixar um pool aberto num processo que vai sair em
// seguida só cria chance de erro.
func comRunner(acao func(*migrations.Runner, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Init(ConfigPath(cmd))
		if err != nil {
			return err
		}

		conexao, err := postgres.AbrirMigracao(cfg.Databases.Postgres, cfg.Migrations)
		if err != nil {
			return err
		}

		runner, err := migrations.NovoRunner(conexao, cfg.Migrations)
		if err != nil {
			return err // o NovoRunner já fechou a conexão
		}
		defer func() { _ = runner.Close() }()

		return acao(runner, args)
	}
}

// imprimirStatus desenha a fotografia do schema.
func imprimirStatus(s migrations.Status) {
	if !s.Aplicada {
		fmt.Println("versão aplicada: nenhuma (banco novo)")
	} else {
		fmt.Printf("versão aplicada: %04d\n", s.Versao)
	}
	if s.Suja {
		fmt.Println("ATENÇÃO: schema SUJO — uma migration falhou no meio. " +
			"Confira as tabelas e use `migrate force <versao>`.")
	}

	fmt.Printf("arquivos: %d | pendentes: %d\n", len(s.Arquivos), len(s.Pendentes))
	for _, arquivo := range s.Pendentes {
		fmt.Printf("  pendente %04d_%s\n", arquivo.Versao, arquivo.Nome)
	}
}
