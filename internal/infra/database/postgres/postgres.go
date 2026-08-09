// Package postgres abre e mantém o pool de conexões do banco transacional.
//
// Duas superfícies, no mesmo desenho do Atila: uma **função pura de conexão**
// (Connect — testável, sem estado global) e um **singleton do processo**
// (InitPostgres/GetDB/Close, em singleton.go).
//
// O Postgres é FATAL para este processo: sem ele não há onde gravar o evento
// que acabou de chegar, e um receptor de eventos que perde eventos em silêncio
// é pior do que um receptor que não sobe.
//
// Um pacote de `infra` não importa outro pacote de `infra`: quem precisa da
// conexão de migração recebe o `*sql.DB` pronto do `cmd/bootstrap`.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"valheim-webhook/internal/pkg/config"
)

// Conn é a conexão do processo: o handle do GORM, o `*sql.DB` por baixo e o
// health check que vigia o pool.
type Conn struct {
	gormDB *gorm.DB
	sqlDB  *sql.DB
	parar  context.CancelFunc
	fim    chan struct{}
	// saudavel guarda o resultado do último ping do health check. É lido pelo
	// `GET /api/status` a cada poucos segundos, então precisa ser uma leitura
	// de memória — nunca um ping novo.
	saudavel atomic.Bool
}

// DB devolve o handle do GORM — é o que os repositories recebem.
func (c *Conn) DB() *gorm.DB {
	if c == nil {
		return nil
	}
	return c.gormDB
}

// SQL devolve o `*sql.DB` por baixo do pool (health check e diagnóstico).
func (c *Conn) SQL() *sql.DB {
	if c == nil {
		return nil
	}
	return c.sqlDB
}

// Ping confere a conexão com prazo curto.
func (c *Conn) Ping(ctx context.Context) error {
	if c == nil || c.sqlDB == nil {
		return ErrNotInitialized
	}
	return c.sqlDB.PingContext(ctx)
}

// Close encerra o health check e o pool. Idempotente.
func (c *Conn) Close() error {
	if c == nil {
		return nil
	}
	if c.parar != nil {
		c.parar()
		<-c.fim
		c.parar = nil
	}
	if c.sqlDB == nil {
		return nil
	}
	err := c.sqlDB.Close()
	c.sqlDB = nil
	return err
}

// TempoDoPing é o prazo de cada ping do health check — curto de propósito: a
// pergunta é "o pool responde agora?", não "o banco consegue responder um dia".
const TempoDoPing = 3 * time.Second

// TempoDeQueryLenta é a partir de quando uma consulta vira linha de log.
//
// 200ms é generoso para o que este produto faz (inserir um evento, listar cem)
// e apertado o bastante para uma consulta que degradou aparecer antes de o
// painel ficar visivelmente lento.
const TempoDeQueryLenta = 200 * time.Millisecond

// Connect abre o pool e sobe o health check.
//
// Função pura: não guarda nada em variável global e pode ser chamada por um
// teste com um banco efêmero. Quem quer o pool do processo usa InitPostgres.
func Connect(cfg config.Postgres) (*Conn, error) {
	dsn := DSN(cfg)

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.New(log.New(os.Stdout, "", 0), logger.Config{
			// Nível `Warn`: query lenta e erro aparecem, o SELECT de cada
			// requisição do painel não.
			LogLevel:      logger.Warn,
			SlowThreshold: TempoDeQueryLenta,
			// "Registro não encontrado" NÃO é erro, e imprimi-lo como se fosse
			// é pior do que ruído: no primeiro boot, as duas consultas que
			// legitimamente não acham nada (a integração ainda não salva, a
			// conta administrativa ainda não criada) saíam em vermelho, com
			// SQL e tudo, no meio das linhas que dizem que deu tudo certo.
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
		// Tudo em UTC dentro do processo. A hora local da máquina do servidor
		// de Valheim só aparece na linha de log que chega — e é convertida na
		// ingestão, uma vez.
		NowFunc:                func() time.Time { return time.Now().UTC() },
		TranslateError:         true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrConexao, redigir(err.Error(), cfg))
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConexao, err)
	}

	sqlDB.SetMaxOpenConns(cfg.Pool.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Pool.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.Pool.ConnMaxLifetime())
	sqlDB.SetConnMaxIdleTime(cfg.Pool.ConnMaxIdleTime())

	// A primeira conexão é aberta AQUI, no boot, e não no primeiro request:
	// banco errado no configs.json deve derrubar o `serve` na hora, com a
	// mensagem certa, em vez de virar um 500 quando o primeiro evento chegar.
	ctx, cancelar := context.WithTimeout(context.Background(), TempoDoPing)
	defer cancelar()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("%w: %s", ErrConexao, redigir(err.Error(), cfg))
	}

	conn := &Conn{gormDB: gormDB, sqlDB: sqlDB, fim: make(chan struct{})}
	conn.saudavel.Store(true) // o ping acima passou
	conn.iniciarHealthCheck(cfg.Pool.HealthInterval())

	slog.Info("[POSTGRES] conectado",
		"host", cfg.Host, "porta", cfg.Port, "banco", cfg.DBName,
		"max_open_conns", cfg.Pool.MaxOpenConns)

	return conn, nil
}

// iniciarHealthCheck vigia o pool em segundo plano.
//
// Ele não reconecta nada (o `database/sql` já faz isso sozinho): existe para
// que a queda do banco apareça no log e no `GET /api/status` ANTES de o
// próximo evento chegar e falhar.
func (c *Conn) iniciarHealthCheck(intervalo time.Duration) {
	if intervalo <= 0 {
		close(c.fim)
		return
	}
	ctx, cancelar := context.WithCancel(context.Background())
	c.parar = cancelar

	go func() {
		defer close(c.fim)
		ticker := time.NewTicker(intervalo)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prazo, cancelarPing := context.WithTimeout(ctx, TempoDoPing)
				err := c.sqlDB.PingContext(prazo)
				cancelarPing()

				// O log sai na TRANSIÇÃO, não a cada ping: banco fora do ar por
				// uma hora seria uma linha de erro a cada 30s, e o que interessa
				// é o instante em que caiu e o instante em que voltou.
				switch anterior := c.saudavel.Swap(err == nil); {
				case err != nil && anterior:
					slog.Error("[POSTGRES] pool sem resposta", "erro", err)
				case err == nil && !anterior:
					slog.Info("[POSTGRES] pool respondendo de novo")
				}
			}
		}
	}()
}

// Saudavel informa se o último ping do health check passou. É o que alimenta o
// health check da API — sem I/O nenhum, porque o `GET /api/status` é chamado a
// cada poucos segundos por quem monitora.
func (c *Conn) Saudavel() bool {
	return c != nil && c.sqlDB != nil && c.saudavel.Load()
}

// DSN monta a string de conexão no formato palavra-chave/valor do libpq.
//
// TimeZone=UTC é parte do contrato: o banco guarda UTC, o painel converte na
// hora de exibir.
func DSN(cfg config.Postgres) string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		cfg.Host, cfg.Port, cfg.User, cfg.Pwd, cfg.DBName, cfg.SSLMode,
	)
}

// redigir tira a senha da mensagem de erro. O driver costuma ecoar a DSN
// inteira quando a conexão falha, e essa mensagem vai para o log.
func redigir(mensagem string, cfg config.Postgres) string {
	if cfg.Pwd == "" {
		return mensagem
	}
	return strings.ReplaceAll(mensagem, cfg.Pwd, "***")
}
