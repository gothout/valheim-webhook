package bootstrap

import (
	"gorm.io/gorm"

	"valheim-webhook/internal/infra/database/postgres"
	"valheim-webhook/internal/infra/jwt"
	"valheim-webhook/internal/infra/notificador/discord"
	"valheim-webhook/internal/pkg/config"
	"valheim-webhook/internal/pkg/errobserve"
)

// abrirInfra abre as conexões e liga as peças de processo compartilhadas.
//
// Ordem: Postgres (FATAL) → JWT (FATAL) → Discord (DEGRADÁVEL). O que é fatal
// devolve erro e nada sobe; o que degrada só loga `[DEGRADADO]` e o produto
// funciona sem aquela parte.
//
// A regra de quem é fatal sai do que o produto É: um receptor de eventos sem
// banco não tem onde guardar o que recebe (fatal) e sem emissor de sessão não
// deixa ninguém abrir o painel (fatal); sem Discord, ele continua gravando
// tudo e mostrando na tela — só não avisa ninguém (degradável).
//
// Devolve o pool e o encerramento. Quem chama registra o `defer fechar()` na
// linha seguinte: em erro, esta função já fechou o que tinha aberto e não
// devolve recurso órfão.
func abrirInfra(cfg *config.Config) (*gorm.DB, func(), error) {
	// A ordem de `fechar` é a mesma que uma pilha de `defer` produziria (LIFO):
	// a fila do observador esvazia ANTES de as conexões saírem, e o publicador
	// do Discord drena o que restou antes de o processo morrer.
	fechar := func() {
		errobserve.Fechar()
		_ = discord.Close()
		_ = postgres.Close()
	}

	// Postgres — fatal.
	db, err := postgres.InitPostgres(cfg.Databases.Postgres)
	if err != nil {
		return nil, nil, err
	}

	// JWT — fatal.
	if _, err := jwt.Init(cfg.Security); err != nil {
		fechar()
		return nil, nil, err
	}

	// Discord — degradável. O que sobe aqui é a configuração do ARQUIVO; se
	// houver configuração salva no painel, o `InitDomains` a aplica por cima
	// logo depois (subdomínio `configuracao/integracao`).
	discord.InitDiscord(cfg.Discord)

	return db, fechar, nil
}
