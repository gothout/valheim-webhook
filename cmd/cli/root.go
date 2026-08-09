// Package cli monta a árvore de comandos do binário (cobra).
//
// Comandos: `serve` (o serviço), `migrate` (schema) e `usuario` (contas do
// painel — o caminho de volta para quem perdeu a senha do administrador).
//
// A árvore é construída por NewRootCommand para ser testável sem estado global;
// o caminho do arquivo de configuração é uma flag persistente da raiz, lida
// pelos subcomandos via ConfigPath.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version é a versão do binário, exibida em `valheim-webhook --version`.
const Version = "0.1.0"

// FlagConfig é o nome da flag persistente com o caminho do configs.json.
const FlagConfig = "config"

// NewRootCommand monta a árvore de comandos.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "valheim-webhook",
		Short: "Receptor de eventos do servidor de Valheim, com painel e Discord",
		Long: "valheim-webhook — recebe as linhas de log do servidor de Valheim " +
			"(hook do valheim-server-docker), guarda o que aconteceu, mostra num painel " +
			"e avisa no Discord.\n\n" +
			"Use `serve` para subir o serviço, `migrate` para o schema e `usuario` para " +
			"as contas do painel.",
		Version: Version,
		// Erro de negócio não deve despejar o help inteiro; a impressão do erro
		// fica a cargo de Execute, para sair com código 1 e mensagem única.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().String(
		FlagConfig, "",
		"caminho do arquivo de configuração (padrão: ./configs.json ou /etc/valheim-webhook/configs.json)",
	)

	root.AddCommand(
		newServeCommand(),
		newMigrateCommand(),
		newUsuarioCommand(),
	)

	return root
}

// Execute roda a CLI e encerra o processo com código 1 em caso de erro.
func Execute() {
	if err := NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

// ConfigPath devolve o valor da flag persistente --config a partir de qualquer
// subcomando (cobra resolve flags herdadas).
func ConfigPath(cmd *cobra.Command) string {
	path, _ := cmd.Flags().GetString(FlagConfig)
	return path
}
