// Package migrations aplica e reverte o schema do PostgreSQL com o
// golang-migrate.
//
// O runner é criado por comando (CLI `valheim-webhook migrate` ou auto-run do
// boot), não é singleton: migration é operação pontual, não recurso do
// processo.
//
// Duas decisões importantes, herdadas do Atila:
//
//   - a conexão vem de fora. Este pacote não sabe abrir Postgres — quem monta a
//     conexão com `lock_timeout`/`statement_timeout` é o
//     `postgres.AbrirMigracao`, e o `cmd/bootstrap` faz a ligação. É o que
//     mantém a regra "um pacote de infra não importa outro";
//   - o Runner ASSUME a conexão recebida (ela é dedicada às migrations) e a
//     fecha no Close.
//
// Cada arquivo roda em uma transação implícita do Postgres: falha no meio
// desfaz o arquivo inteiro, sem meio-termo.
package migrations

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"valheim-webhook/internal/pkg/config"
)

// TabelaDeControle é onde o golang-migrate guarda a versão aplicada.
const TabelaDeControle = "schema_migrations"

// Nomes dos drivers do golang-migrate.
const (
	nomeDaFonte = "iofs"
	nomeDoBanco = "pgx5"
)

// Runner executa as operações de migration sobre um banco.
type Runner struct {
	m        *migrate.Migrate
	caminho  string
	arquivos []Arquivo
}

// Status é a fotografia do schema: o que está aplicado e o que falta.
type Status struct {
	// Versao é a última migration aplicada (0 quando nenhuma foi).
	Versao uint
	// Aplicada distingue "nenhuma migration aplicada" de "aplicada a versão 0".
	Aplicada bool
	// Suja indica que uma migration falhou no meio.
	Suja bool
	// Arquivos são todas as migrations do diretório, ordenadas por versão.
	Arquivos []Arquivo
	// Pendentes são as migrations com versão acima da aplicada.
	Pendentes []Arquivo
}

// NovoRunner monta o runner sobre uma conexão já aberta e dedicada.
//
// Em caso de erro, a conexão recebida é fechada — quem chamou não fica com um
// *sql.DB órfão.
func NovoRunner(db *sql.DB, cfg config.Migrations) (*Runner, error) {
	if db == nil {
		return nil, ErrConexaoNula
	}

	caminho, err := filepath.Abs(strings.TrimSpace(cfg.Path))
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w: %s: %v", ErrDiretorio, cfg.Path, err)
	}

	arquivos, err := Validar(caminho)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	fonte, err := iofs.New(os.DirFS(caminho), ".")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w: %s: %v", ErrDiretorio, caminho, err)
	}

	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{
		MigrationsTable: TabelaDeControle,
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("driver de migração: %w", err)
	}

	m, err := migrate.NewWithInstance(nomeDaFonte, fonte, nomeDoBanco, driver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("runner de migração: %w", err)
	}

	return &Runner{m: m, caminho: caminho, arquivos: arquivos}, nil
}

// Up aplica todas as migrations pendentes. Nada pendente não é erro.
func (r *Runner) Up() error {
	if err := r.m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return r.traduzir(err)
	}
	return nil
}

// Down desfaz `n` migrations (n >= 1). Não existe "desfazer tudo" por atalho:
// apagar o histórico inteiro do servidor precisa ser um pedido explícito, uma
// versão de cada vez.
func (r *Runner) Down(n int) error {
	if n < 1 {
		return fmt.Errorf("migrate down: informe quantas versões desfazer (recebi %d)", n)
	}
	if err := r.m.Steps(-n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return r.traduzir(err)
	}
	return nil
}

// Goto leva o schema até a versão informada (para cima ou para baixo).
func (r *Runner) Goto(versao uint) error {
	if err := r.m.Migrate(versao); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return r.traduzir(err)
	}
	return nil
}

// Force marca a versão como aplicada SEM rodar nada. É o conserto de um schema
// sujo, depois de o operador ter conferido o estado real das tabelas.
func (r *Runner) Force(versao int) error {
	if err := r.m.Force(versao); err != nil {
		return r.traduzir(err)
	}
	return nil
}

// Status devolve a fotografia do schema.
func (r *Runner) Status() (Status, error) {
	s := Status{Arquivos: r.arquivos}

	versao, suja, err := r.m.Version()
	switch {
	case errors.Is(err, migrate.ErrNilVersion):
		// Banco novo: nenhuma migration aplicada. Tudo é pendente.
	case err != nil:
		return s, r.traduzir(err)
	default:
		s.Versao, s.Suja, s.Aplicada = versao, suja, true
	}

	for _, arquivo := range r.arquivos {
		if !s.Aplicada || arquivo.Versao > s.Versao {
			s.Pendentes = append(s.Pendentes, arquivo)
		}
	}
	return s, nil
}

// Close encerra a fonte e a conexão assumida pelo runner.
func (r *Runner) Close() error {
	erroFonte, erroBanco := r.m.Close()
	return errors.Join(erroFonte, erroBanco)
}

// traduzir converte o erro do golang-migrate no vocabulário deste pacote.
func (r *Runner) traduzir(err error) error {
	var sujo migrate.ErrDirty
	if errors.As(err, &sujo) {
		return fmt.Errorf("%w: versão %d. Confira as tabelas e rode "+
			"`valheim-webhook migrate force <versao>` com a versão realmente aplicada",
			ErrSchemaSujo, sujo.Version)
	}
	return err
}

// Caminho devolve o diretório de migrations resolvido (log e mensagens).
func (r *Runner) Caminho() string { return r.caminho }

// Aplicar é o atalho do boot: sobe o schema até a última versão e loga o que
// fez. Existe para o `serve` não repetir as cinco linhas do `migrate up`.
func Aplicar(db *sql.DB, cfg config.Migrations) error {
	runner, err := NovoRunner(db, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = runner.Close() }()

	antes, err := runner.Status()
	if err != nil {
		return err
	}
	if len(antes.Pendentes) == 0 {
		slog.Info("[MIGRATIONS] schema em dia", "versao", antes.Versao, "caminho", runner.Caminho())
		return nil
	}

	slog.Info("[MIGRATIONS] aplicando pendentes",
		"quantidade", len(antes.Pendentes), "de", antes.Versao, "caminho", runner.Caminho())

	if err := runner.Up(); err != nil {
		return err
	}

	depois, err := runner.Status()
	if err != nil {
		return err
	}
	slog.Info("[MIGRATIONS] schema atualizado", "versao", depois.Versao)
	return nil
}
