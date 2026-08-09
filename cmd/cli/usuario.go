package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"valheim-webhook/internal/iam/domain/identidade/usuario"
	"valheim-webhook/internal/infra/database/postgres"
	"valheim-webhook/internal/pkg/config"
	"valheim-webhook/internal/pkg/papel"
)

// newUsuarioCommand monta `usuario listar|criar|senha`.
//
// Este comando existe por um motivo específico: o painel exige login, e a
// gestão de usuários exige um administrador. Se o único administrador perder a
// senha, sem isto o conserto seria escrever um bcrypt à mão no Postgres. Ele é
// a porta dos fundos — e roda apenas em quem tem acesso ao servidor.
func newUsuarioCommand() *cobra.Command {
	comando := &cobra.Command{
		Use:   "usuario",
		Short: "Gerencia as contas do painel pela linha de comando",
	}

	comando.AddCommand(comandoListar(), comandoCriar(), comandoSenha())
	return comando
}

func comandoListar() *cobra.Command {
	return &cobra.Command{
		Use:   "listar",
		Short: "Lista as contas do painel",
		Args:  cobra.NoArgs,
		RunE: comUsuarios(func(cmd *cobra.Command, service usuario.Service, _ []string) error {
			usuarios, err := service.Listar(cmd.Context())
			if err != nil {
				return err
			}
			if len(usuarios) == 0 {
				fmt.Println("nenhuma conta cadastrada (o `serve` cria a primeira no boot)")
				return nil
			}
			for _, u := range usuarios {
				situacao := "ativo"
				if !u.Ativo {
					situacao = "inativo"
				}
				fmt.Printf("%-32s %-24s %-14s %s\n", u.Email, u.Nome, u.Papel, situacao)
			}
			return nil
		}),
	}
}

func comandoCriar() *cobra.Command {
	comando := &cobra.Command{
		Use:   "criar",
		Short: "Cria uma conta",
		Args:  cobra.NoArgs,
		RunE: comUsuarios(func(cmd *cobra.Command, service usuario.Service, _ []string) error {
			email, _ := cmd.Flags().GetString("email")
			nome, _ := cmd.Flags().GetString("nome")
			senha, _ := cmd.Flags().GetString("senha")
			papelPedido, _ := cmd.Flags().GetString("papel")

			p, ok := papel.De(papelPedido)
			if !ok {
				return fmt.Errorf("papel inválido: %q (use admin ou visualizador)", papelPedido)
			}
			if strings.TrimSpace(nome) == "" {
				nome = email
			}

			u, err := service.Criar(cmd.Context(), nome, email, senha, p)
			if err != nil {
				return err
			}
			fmt.Printf("criado: %s (%s)\n", u.Email, u.Papel)
			return nil
		}),
	}

	comando.Flags().String("email", "", "e-mail da conta (obrigatório)")
	comando.Flags().String("nome", "", "nome exibido (padrão: o e-mail)")
	comando.Flags().String("senha", "", "senha inicial, de 8 a 72 caracteres (obrigatório)")
	comando.Flags().String("papel", string(papel.Visualizador), "admin | visualizador")
	_ = comando.MarkFlagRequired("email")
	_ = comando.MarkFlagRequired("senha")

	return comando
}

func comandoSenha() *cobra.Command {
	comando := &cobra.Command{
		Use:   "senha",
		Short: "Troca a senha de uma conta (recuperação de acesso)",
		Args:  cobra.NoArgs,
		RunE: comUsuarios(func(cmd *cobra.Command, service usuario.Service, _ []string) error {
			email, _ := cmd.Flags().GetString("email")
			senha, _ := cmd.Flags().GetString("senha")

			usuarios, err := service.Listar(cmd.Context())
			if err != nil {
				return err
			}
			for _, u := range usuarios {
				if strings.EqualFold(u.Email, strings.TrimSpace(email)) {
					if err := service.TrocarSenha(cmd.Context(), u.UUID, senha); err != nil {
						return err
					}
					fmt.Printf("senha trocada para %s\n", u.Email)
					return nil
				}
			}
			return fmt.Errorf("conta não encontrada: %s", email)
		}),
	}

	comando.Flags().String("email", "", "e-mail da conta (obrigatório)")
	comando.Flags().String("senha", "", "senha nova, de 8 a 72 caracteres (obrigatório)")
	_ = comando.MarkFlagRequired("email")
	_ = comando.MarkFlagRequired("senha")

	return comando
}

// comUsuarios abre só o que estes comandos precisam: configuração, Postgres e o
// subdomínio de usuários.
//
// Não é o boot inteiro de propósito — trocar uma senha não deveria exigir que o
// Discord, o painel e a ingestão subissem junto.
func comUsuarios(acao func(*cobra.Command, usuario.Service, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Init(ConfigPath(cmd))
		if err != nil {
			return err
		}

		db, err := postgres.InitPostgres(cfg.Databases.Postgres)
		if err != nil {
			return err
		}
		defer func() { _ = postgres.Close() }()

		if _, err := usuario.New(db); err != nil {
			return err
		}

		return acao(cmd, usuario.MustUse().Service, args)
	}
}
