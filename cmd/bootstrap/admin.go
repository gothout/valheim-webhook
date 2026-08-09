package bootstrap

import (
	"context"
	"log/slog"
	"strings"

	"valheim-webhook/internal/iam/domain/identidade/usuario"
	"valheim-webhook/internal/pkg/config"
)

// garantirAdministrador cria a conta inicial quando a instalação é nova.
//
// Sem isto, um painel com login e gestão de usuários não teria como ser usado
// pela primeira vez: alguém teria de inserir um bcrypt à mão no Postgres.
//
// A senha sorteada é impressa UMA vez, no log do boot, com destaque. É o
// desenho seguro: quem não escolheu senha não fica com `admin/admin`, e quem
// subiu o serviço encontra a senha no `docker logs` da primeira execução.
func garantirAdministrador(ctx context.Context, cfg *config.Config) error {
	service := usuario.MustUse().Service

	criado, senhaSorteada, err := service.GarantirAdministrador(
		ctx, cfg.Admin.Nome, cfg.Admin.Email, cfg.Admin.Senha)
	if err != nil {
		return err
	}
	if criado == nil {
		return nil // já havia usuários: instalação não é nova
	}

	if senhaSorteada == "" {
		slog.Info("[BOOTSTRAP] conta administrativa criada com a senha do configs.json",
			"email", criado.Email)
		return nil
	}

	// Bloco visualmente destacado de propósito: esta linha passa UMA vez na
	// vida da instalação, no meio de dezenas de outras, e perdê-la significa
	// não conseguir entrar.
	slog.Warn(strings.Join([]string{
		"",
		"┌──────────────────────────────────────────────────────────────┐",
		"│  CONTA ADMINISTRATIVA CRIADA — anote a senha, ela não volta  │",
		"└──────────────────────────────────────────────────────────────┘",
		"    e-mail: " + criado.Email,
		"     senha: " + senhaSorteada,
		"",
		"Troque-a no painel, em Administração › Minha senha.",
		"Para escolher a senha desde o começo, use VALHEIM_WEBHOOK_ADMIN_SENHA.",
		"",
	}, "\n"))

	return nil
}
