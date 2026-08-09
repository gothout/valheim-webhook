package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CaminhosPadrao são procurados, nessa ordem, quando `--config` não é passado.
var CaminhosPadrao = []string{
	"configs.json",
	"/etc/valheim-webhook/configs.json",
}

// PrefixoEnv é o prefixo das variáveis de ambiente que sobrescrevem o arquivo.
const PrefixoEnv = "VALHEIM_WEBHOOK_"

// ErrArquivoNaoEncontrado indica que nenhum caminho candidato existe.
var ErrArquivoNaoEncontrado = errors.New("arquivo de configuração não encontrado")

// Environment carrega a configuração: lê o arquivo, aplica os defaults, deixa
// o ambiente sobrescrever e valida o resultado.
//
// `path` vazio dispara a busca em CaminhosPadrao. Erro aqui é fatal para todo
// comando — nenhum deles faz sentido sem configuração válida.
func Environment(path string) (*Config, error) {
	arquivo, err := resolverCaminho(path)
	if err != nil {
		return nil, err
	}

	conteudo, err := os.ReadFile(arquivo)
	if err != nil {
		return nil, fmt.Errorf("ler %s: %w", arquivo, err)
	}

	cfg := &Config{}
	// DisallowUnknownFields fica de fora de propósito: chave a mais no arquivo
	// (de uma versão futura, ou de um comentário mal apagado) não é motivo para
	// o processo não subir.
	if err := json.Unmarshal(conteudo, cfg); err != nil {
		return nil, fmt.Errorf("interpretar %s: %w", arquivo, err)
	}
	cfg.arquivo = arquivo

	aplicarDefaults(cfg)
	aplicarAmbiente(cfg)

	if err := Validar(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", arquivo, err)
	}
	return cfg, nil
}

// resolverCaminho devolve o caminho absoluto do arquivo a carregar.
func resolverCaminho(path string) (string, error) {
	if p := strings.TrimSpace(path); p != "" {
		absoluto, err := filepath.Abs(p)
		if err != nil {
			return "", fmt.Errorf("resolver caminho %q: %w", p, err)
		}
		if _, err := os.Stat(absoluto); err != nil {
			return "", fmt.Errorf("%w: %s", ErrArquivoNaoEncontrado, absoluto)
		}
		return absoluto, nil
	}

	for _, candidato := range CaminhosPadrao {
		absoluto, err := filepath.Abs(candidato)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absoluto); err == nil {
			return absoluto, nil
		}
	}
	return "", fmt.Errorf("%w: procurei em %s (copie o configs_example.json)",
		ErrArquivoNaoEncontrado, strings.Join(CaminhosPadrao, ", "))
}

// aplicarDefaults preenche o que ficou em branco com valores que funcionam.
//
// Configuração mínima é um objetivo aqui: quem só quer receber eventos deveria
// precisar declarar o banco e o destino do Discord, nada mais.
func aplicarDefaults(c *Config) {
	if c.App.Name == "" {
		c.App.Name = "valheim-webhook"
	}
	if c.App.Env == "" {
		c.App.Env = "dev"
	}
	if c.App.Version == "" {
		c.App.Version = "0.1.0"
	}
	if c.Server.HTTP.Port == 0 {
		c.Server.HTTP.Port = 6060
	}
	if c.Security.SessaoHoras <= 0 {
		c.Security.SessaoHoras = 168 // uma semana
	}
	if c.Admin.Email == "" {
		c.Admin.Email = "admin@valheim.local"
	}
	if c.Admin.Nome == "" {
		c.Admin.Nome = "Administrador"
	}
	if c.Ingestao.MaxBodyBytes <= 0 {
		c.Ingestao.MaxBodyBytes = 64 << 10 // 64 KiB
	}
	if c.Ingestao.Servidor == "" {
		c.Ingestao.Servidor = "valheim"
	}

	if c.Databases.Postgres.Port == "" {
		c.Databases.Postgres.Port = "5432"
	}
	if c.Databases.Postgres.SSLMode == "" {
		c.Databases.Postgres.SSLMode = "disable"
	}
	pool := &c.Databases.Postgres.Pool
	if pool.MaxOpenConns <= 0 {
		pool.MaxOpenConns = 10
	}
	if pool.MaxIdleConns <= 0 {
		pool.MaxIdleConns = 5
	}
	if pool.ConnMaxLifetimeMin <= 0 {
		pool.ConnMaxLifetimeMin = 5
	}
	if pool.ConnMaxIdleTimeMin <= 0 {
		pool.ConnMaxIdleTimeMin = 2
	}
	if pool.HealthIntervalSec <= 0 {
		pool.HealthIntervalSec = 30
	}

	if c.Discord.TimeoutSec <= 0 {
		c.Discord.TimeoutSec = 10
	}
	if c.Discord.Fila <= 0 {
		c.Discord.Fila = 256
	}
	if c.Discord.Username == "" {
		c.Discord.Username = "Huginn"
	}

	if c.Migrations.Path == "" {
		c.Migrations.Path = "db/migrations"
	}
	if c.Migrations.LockTimeoutSec <= 0 {
		c.Migrations.LockTimeoutSec = 5
	}
	if c.Migrations.StatementTimeoutMin <= 0 {
		c.Migrations.StatementTimeoutMin = 10
	}

	if c.Painel.Titulo == "" {
		c.Painel.Titulo = "Valheim — " + c.Ingestao.Servidor
	}
	if c.Painel.EventosNoFeed <= 0 {
		c.Painel.EventosNoFeed = 100
	}
}

// aplicarAmbiente deixa o ambiente sobrescrever o arquivo.
//
// A lista é curta e explícita de propósito: são as chaves que não deveriam
// estar num arquivo versionado (segredos) e as que mudam por instalação
// (endereço do banco, porta). O resto vive no configs.json.
func aplicarAmbiente(c *Config) {
	texto(PrefixoEnv+"DISCORD_WEBHOOK_URL", &c.Discord.WebhookURL)
	texto(PrefixoEnv+"DISCORD_BOT_TOKEN", &c.Discord.BotToken)
	texto(PrefixoEnv+"DISCORD_CHANNEL_ID", &c.Discord.ChannelID)
	texto(PrefixoEnv+"INGEST_TOKEN", &c.Ingestao.Token)
	texto(PrefixoEnv+"JWT_SECRET", &c.Security.JWTSecret)
	texto(PrefixoEnv+"ADMIN_EMAIL", &c.Admin.Email)
	texto(PrefixoEnv+"ADMIN_SENHA", &c.Admin.Senha)

	texto(PrefixoEnv+"PG_HOST", &c.Databases.Postgres.Host)
	texto(PrefixoEnv+"PG_PORT", &c.Databases.Postgres.Port)
	texto(PrefixoEnv+"PG_USER", &c.Databases.Postgres.User)
	texto(PrefixoEnv+"PG_PWD", &c.Databases.Postgres.Pwd)
	texto(PrefixoEnv+"PG_DB", &c.Databases.Postgres.DBName)
	texto(PrefixoEnv+"PG_SSLMODE", &c.Databases.Postgres.SSLMode)

	texto(PrefixoEnv+"SERVIDOR", &c.Ingestao.Servidor)
	texto(PrefixoEnv+"PAINEL_TITULO", &c.Painel.Titulo)

	// Publicação em domínio próprio (ex.: base `atila.cloud`, painel em
	// `valheim.atila.cloud`). Ver o comentário de App.BaseDomain.
	texto(PrefixoEnv+"BASE_DOMAIN", &c.App.BaseDomain)
	texto(PrefixoEnv+"PAINEL_DOMINIO", &c.Painel.Dominio)
	texto(PrefixoEnv+"COOKIE_DOMINIO", &c.Security.CookieDominio)
	booleano(PrefixoEnv+"COOKIE_SEGURO", &c.Security.CookieSeguro)

	// Atrás de proxy reverso, o IP real do cliente vem no X-Forwarded-For — e
	// o Gin só o aceita se a faixa do proxy estiver listada.
	lista(PrefixoEnv+"TRUSTED_PROXY", &c.Server.HTTP.TrustedProxy)
	lista(PrefixoEnv+"CORS_ORIGENS", &c.Server.HTTP.CORS.AllowedOrigins)

	inteiro(PrefixoEnv+"HTTP_PORT", &c.Server.HTTP.Port)
}

// texto sobrescreve o destino quando a variável existe e não está vazia.
func texto(chave string, destino *string) {
	if valor, definida := os.LookupEnv(chave); definida && strings.TrimSpace(valor) != "" {
		*destino = strings.TrimSpace(valor)
	}
}

// booleano sobrescreve o destino com as formas que uma variável de ambiente
// costuma usar. Valor irreconhecível é ignorado com aviso — nunca interpretado
// como `false`, que seria desligar uma proteção por causa de um erro de digitação.
func booleano(chave string, destino *bool) {
	valor, definida := os.LookupEnv(chave)
	if !definida || strings.TrimSpace(valor) == "" {
		return
	}
	switch strings.ToLower(strings.TrimSpace(valor)) {
	case "1", "true", "sim", "yes", "on":
		*destino = true
	case "0", "false", "nao", "não", "no", "off":
		*destino = false
	default:
		fmt.Fprintf(os.Stderr, "aviso: %s=%q não é sim/não; mantendo o valor do arquivo\n", chave, valor)
	}
}

// lista sobrescreve o destino com uma lista separada por vírgula.
func lista(chave string, destino *[]string) {
	valor, definida := os.LookupEnv(chave)
	if !definida {
		return
	}

	var itens []string
	for _, pedaco := range strings.Split(valor, ",") {
		if limpo := strings.TrimSpace(pedaco); limpo != "" {
			itens = append(itens, limpo)
		}
	}
	// Variável definida e VAZIA é um pedido explícito de "lista vazia" — é como
	// se apaga uma lista herdada do arquivo sem editar o arquivo.
	*destino = itens
}

// inteiro sobrescreve o destino quando a variável existe e é um número.
//
// Valor inválido é ignorado com aviso em vez de derrubar o boot: variável de
// ambiente errada costuma ser dedo no docker-compose, e o arquivo já traz um
// valor bom.
func inteiro(chave string, destino *int) {
	valor, definida := os.LookupEnv(chave)
	if !definida || strings.TrimSpace(valor) == "" {
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(valor))
	if err != nil {
		fmt.Fprintf(os.Stderr, "aviso: %s=%q não é um número; mantendo o valor do arquivo\n", chave, valor)
		return
	}
	*destino = n
}

// Validar recusa a configuração que não permitiria o processo funcionar.
//
// Só o que é FATAL entra aqui. Discord sem destino, por exemplo, não é erro: o
// receptor grava os eventos e serve o painel do mesmo jeito, e o boot avisa.
func Validar(c *Config) error {
	if c == nil {
		return errors.New("configuração nula")
	}
	if c.Server.HTTP.Port < 1 || c.Server.HTTP.Port > 65535 {
		return fmt.Errorf("server.http.port inválido: %d", c.Server.HTTP.Port)
	}

	pg := c.Databases.Postgres
	if strings.TrimSpace(pg.Host) == "" {
		return errors.New("databases.postgres.host é obrigatório")
	}
	if strings.TrimSpace(pg.User) == "" {
		return errors.New("databases.postgres.user é obrigatório")
	}
	if strings.TrimSpace(pg.DBName) == "" {
		return errors.New("databases.postgres.db_name é obrigatório")
	}
	if _, err := strconv.Atoi(strings.TrimSpace(pg.Port)); err != nil {
		return fmt.Errorf("databases.postgres.port inválido: %q", pg.Port)
	}

	if c.IsProduction() {
		if strings.TrimSpace(c.Ingestao.Token) == "" {
			return errors.New("ingestao.token é obrigatório quando app.env=prod " +
				"(a rota POST /eventos ficaria aberta na internet)")
		}
		if strings.TrimSpace(c.Security.JWTSecret) == "" {
			return errors.New("security.jwt_secret é obrigatório quando app.env=prod " +
				"(sem ele, todo reinício desloga todo mundo)")
		}
	}
	return nil
}
