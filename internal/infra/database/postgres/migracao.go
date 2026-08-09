package postgres

import (
	"database/sql"
	"fmt"

	// Registra o driver `pgx` no database/sql. É o mesmo driver que o pool do
	// GORM usa por baixo — não entra um segundo cliente de Postgres no binário.
	_ "github.com/jackc/pgx/v5/stdlib"

	"valheim-webhook/internal/pkg/config"
)

// AbrirMigracao abre uma conexão DEDICADA às migrations.
//
// Ela é separada do pool da aplicação por três motivos:
//
//   - `lock_timeout`: migration que não consegue o lock da tabela falha rápido
//     em vez de ficar na fila segurando as conexões seguintes;
//   - `statement_timeout`: migration que trava não trava o processo para sempre;
//   - uma conexão só (`SetMaxOpenConns(1)`): o golang-migrate usa uma conexão
//     e o lock de migração é por sessão — mais de uma seria uma segunda sessão
//     disputando o mesmo lock.
//
// Quem chama assume a conexão e a fecha (o Runner das migrations faz isso).
func AbrirMigracao(pg config.Postgres, m config.Migrations) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s options='-c lock_timeout=%d -c statement_timeout=%d'",
		DSN(pg),
		m.LockTimeout().Milliseconds(),
		m.StatementTimeout().Milliseconds(),
	)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w (migração): %s", ErrConexao, redigir(err.Error(), pg))
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w (migração): %s", ErrConexao, redigir(err.Error(), pg))
	}
	return db, nil
}
