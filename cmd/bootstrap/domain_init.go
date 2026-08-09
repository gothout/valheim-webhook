package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"valheim-webhook/internal/iam/application/identidade/auth"
	"valheim-webhook/internal/iam/domain/identidade/usuario"
	"valheim-webhook/internal/iam/middleware"
	"valheim-webhook/internal/infra/jwt"
	"valheim-webhook/internal/pkg/config"
	"valheim-webhook/internal/pkg/log/memoria"
	"valheim-webhook/internal/pkg/tempo_real"
	"valheim-webhook/internal/valheim/application/eventos/ingestao"
	"valheim-webhook/internal/valheim/domain/configuracao/integracao"
	"valheim-webhook/internal/valheim/domain/eventos/evento"
	"valheim-webhook/internal/valheim/domain/mundo/jogador"
	"valheim-webhook/internal/valheim/domain/observabilidade/trilhas"
)

// InitDomains inicializa os contêineres de domínio na ordem de dependência:
// primeiro o sistema transversal `iam` (de onde sai o middleware que todos os
// controllers usam), depois a vertente `valheim`.
//
// Roda DEPOIS das migrations e ANTES do servidor HTTP: um subdomínio cujo `New`
// falha derruba o boot em vez de responder 500 em produção.
func InitDomains(ctx context.Context, db *gorm.DB, cfg *config.Config, hub *tempo_real.Hub) error {
	if err := initIamDomain(db, cfg); err != nil {
		return fmt.Errorf("iam: %w", err)
	}
	if err := initValheimDomain(ctx, db, cfg, hub); err != nil {
		return fmt.Errorf("valheim: %w", err)
	}
	return nil
}

// initIamDomain registra identidade, sessão e a cadeia de autorização.
//
// A ordem aqui é obrigatória e não é estética:
//
//  1. `usuario` — é quem guarda as contas;
//  2. `middleware` — precisa do serviço de usuários para CONFERIR a conta a
//     cada requisição, e precisa existir antes de qualquer `Routes()`, porque
//     todo controller chama `middleware.MustUse()` ao registrar as rotas;
//  3. `auth` — costura a conta com o emissor de token.
func initIamDomain(db *gorm.DB, cfg *config.Config) error {
	if _, err := usuario.New(db); err != nil {
		return fmt.Errorf("identidade/usuario: %w", err)
	}
	contas := usuario.MustUse().Service

	emissor := jwt.MustUse()

	if _, err := middleware.New(middleware.Dependencias{
		Sessao:     sessaoJWT{sessao: emissor},
		Conferente: conferenteDeConta{service: contas},
	}); err != nil {
		return fmt.Errorf("middleware: %w", err)
	}

	if _, err := auth.New(auth.Dependencias{
		Usuarios: contasParaAuth{service: contas},
		Emissor:  emissor,
	}, auth.Opcoes{
		CookieSeguro:  cfg.Security.CookieSeguro,
		CookieDominio: cfg.Security.CookieDominio,
	}); err != nil {
		return fmt.Errorf("identidade/auth: %w", err)
	}

	slog.Info("[BOOTSTRAP-DI] Contêiner IAM/Identidade inicializado.")
	return nil
}

// initValheimDomain registra os subdomínios do produto.
//
// Ordem: os dois donos de tabela (`evento`, `jogador`), a configuração (que a
// ingestão consulta para saber o que notificar), a trilha de log e, por último,
// a ingestão — que depende de todos os anteriores.
func initValheimDomain(ctx context.Context, db *gorm.DB, cfg *config.Config, hub *tempo_real.Hub) error {
	if _, err := evento.New(db); err != nil {
		return fmt.Errorf("eventos/evento: %w", err)
	}
	if _, err := jogador.New(db); err != nil {
		return fmt.Errorf("mundo/jogador: %w", err)
	}

	if _, err := integracao.New(db, integracao.Dependencias{
		Base:         cfg.Discord,
		TiposValidos: textoDosTipos(evento.TiposConhecidos()),
		TiposPadrao:  textoDosTipos(evento.TiposNotificaveisPadrao()),
	}); err != nil {
		return fmt.Errorf("configuracao/integracao: %w", err)
	}

	// A configuração salva pelo painel vence a do arquivo — e passa a valer
	// AGORA, não no próximo reinício. Falha aqui não derruba o boot: o
	// publicador continua com o que veio do `configs.json`, e o painel mostra o
	// erro na tela de integração.
	if err := integracao.MustUse().Service.Aplicar(ctx); err != nil {
		slog.Warn("[BOOTSTRAP-DI] não foi possível aplicar a integração salva; "+
			"vale o que está no configs.json", "erro", err)
	}

	if _, err := trilhas.New(trilhas.Dependencias{Fonte: memoria.Use()}); err != nil {
		return fmt.Errorf("observabilidade/trilhas: %w", err)
	}

	if _, err := ingestao.New(ingestao.Dependencias{
		Eventos:   eventosParaIngestao{service: evento.MustUse().Service},
		Jogadores: jogadoresParaIngestao{service: jogador.MustUse().Service},
		Politica:  integracao.MustUse().Service,
		Hub:       hub,
		Servidor:  cfg.Ingestao.Servidor,
	}, ingestao.Opcoes{
		Token:        cfg.Ingestao.Token,
		MaxBodyBytes: cfg.Ingestao.MaxBodyBytes,
	}); err != nil {
		return fmt.Errorf("eventos/ingestao: %w", err)
	}

	slog.Info("[BOOTSTRAP-DI] Contêiner Valheim/Eventos inicializado.")
	return nil
}

// textoDosTipos converte o vocabulário do subdomínio de eventos em strings.
//
// A conversão acontece AQUI porque é aqui que os dois lados se encontram: o
// dono do vocabulário (`evento`) e quem só precisa validar contra ele
// (`integracao`), que são irmãos e não podem se importar.
func textoDosTipos(tipos []evento.Tipo) []string {
	lista := make([]string, 0, len(tipos))
	for _, tipo := range tipos {
		lista = append(lista, string(tipo))
	}
	return lista
}
