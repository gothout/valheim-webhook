// Package bootstrap é a composição do processo: config → log → infraestrutura
// → migrations → domínios → HTTP.
//
// É o único lugar do repositório onde dois pacotes de `infra` se encontram, e
// onde um subdomínio enxerga outro (pelos adaptadores de `adaptadores.go`).
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"valheim-webhook/cmd/server"
	"valheim-webhook/cmd/server/routes"
	"valheim-webhook/internal/infra/database/migrations"
	"valheim-webhook/internal/infra/database/postgres"
	"valheim-webhook/internal/infra/notificador/discord"
	"valheim-webhook/internal/pkg/config"
	"valheim-webhook/internal/pkg/log/memoria"
	"valheim-webhook/internal/pkg/tempo_real"
	"valheim-webhook/internal/valheim/application/eventos/ingestao"
)

// CapacidadeDoFluxo é o buffer por painel aberto no barramento de tempo real.
const CapacidadeDoFluxo = 64

// Start sobe o serviço e serve até o contexto ser cancelado.
func Start(ctx context.Context, configPath string) error {
	cfg, err := config.Init(configPath)
	if err != nil {
		return err
	}

	// O anel de log é instalado logo depois da configuração e ANTES de tudo o
	// mais: o painel de logs só vale a pena se contiver o boot, que é onde
	// aparecem "conectado", "[DEGRADADO]" e "migrations aplicadas".
	memoria.Init(memoria.CapacidadePadrao, slog.LevelInfo)

	slog.Info("[BOOTSTRAP] configuração carregada",
		"arquivo", cfg.Arquivo(), "env", cfg.App.Env, "versao", cfg.App.Version)

	// Modo do Gin é estado global do processo: definido aqui, uma vez, e nunca
	// dentro do pacote de rotas (que os testes também montam).
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	db, fechar, err := abrirInfra(cfg)
	if err != nil {
		return err
	}
	defer fechar()

	// Migrations antes dos domínios: subdomínio que sobe consultando tabela
	// inexistente falharia no primeiro request, não no boot.
	if cfg.Migrations.AutoRun {
		if err := aplicarMigrations(cfg); err != nil {
			return err
		}
	} else {
		slog.Warn("[MIGRATIONS] auto_run desligado: rode `valheim-webhook migrate up` à mão")
	}

	hub := tempo_real.NovoHub(CapacidadeDoFluxo)
	defer hub.Fechar()

	if err := InitDomains(ctx, db, cfg, hub); err != nil {
		return err
	}

	if err := garantirAdministrador(ctx, cfg); err != nil {
		return fmt.Errorf("conta administrativa inicial: %w", err)
	}

	engine, err := routes.NovoEngine(cfg, routes.Opcoes{Sondas: sondas()})
	if err != nil {
		return err
	}

	slog.Info("[BOOTSTRAP] pronto",
		"porta", cfg.Server.HTTP.Port,
		"painel", cfg.URLPublica(),
		"ingestao", cfg.URLPublica()+ingestao.RotaEventos,
		"status", cfg.URLPublica()+routes.RotaStatus,
	)
	avisarSobrePublicacao(cfg)

	if err := server.Novo(cfg, engine).Rodar(ctx); err != nil {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	return nil
}

// avisarSobrePublicacao cobra, no boot, as duas coisas que se esquece ao trocar
// `localhost:6060` por um domínio de verdade.
//
// Nenhuma delas impede o processo de subir — as duas produzem sintomas
// confusos: cookie sem `Secure` viaja em claro num painel que está em HTTPS, e
// `X-Forwarded-For` de proxy não listado faz TODO log de acesso registrar o IP
// do proxy, como se o mundo inteiro fosse um cliente só.
func avisarSobrePublicacao(cfg *config.Config) {
	publicado := strings.TrimSpace(cfg.Painel.Dominio) != "" ||
		strings.TrimSpace(cfg.App.BaseDomain) != ""
	if !publicado {
		return
	}

	if !cfg.Security.CookieSeguro {
		slog.Warn("[BOOTSTRAP] painel publicado em domínio, mas security.cookie_seguro=false: " +
			"ligue-o (VALHEIM_WEBHOOK_COOKIE_SEGURO=true) para o cookie de sessão " +
			"só trafegar em HTTPS")
	}
	if len(cfg.Server.HTTP.TrustedProxy) == 0 {
		slog.Warn("[BOOTSTRAP] nenhum proxy confiável listado: atrás de nginx/Caddy, o IP " +
			"de todo request no log será o do proxy. Liste a faixa dele em " +
			"VALHEIM_WEBHOOK_TRUSTED_PROXY")
	}
}

// aplicarMigrations sobe o schema até a última versão.
//
// A conexão de migração é DEDICADA (com `lock_timeout` e `statement_timeout`) e
// é fechada pelo runner — ela não é o pool da aplicação.
func aplicarMigrations(cfg *config.Config) error {
	conexao, err := postgres.AbrirMigracao(cfg.Databases.Postgres, cfg.Migrations)
	if err != nil {
		return err
	}
	return migrations.Aplicar(conexao, cfg.Migrations)
}

// sondas liga o health check ao estado real das dependências. Fica aqui, e não
// em `routes`, porque o pacote de rotas não importa `infra`.
//
// Nenhuma sonda faz I/O: o health check é chamado a cada poucos segundos pelo
// Docker e não pode virar carga no banco.
func sondas() map[string]routes.Sonda {
	return map[string]routes.Sonda{
		"postgres": estado(postgres.Disponivel),
		"discord":  estado(discord.Disponivel),
	}
}

// estado adapta um "está disponível?" para o vocabulário do health check.
func estado(disponivel func() bool) routes.Sonda {
	return func() string {
		if disponivel() {
			return routes.EstadoOK
		}
		return routes.EstadoDegradado
	}
}
