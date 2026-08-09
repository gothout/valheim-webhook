// Package config carrega e valida a configuração do valheim-webhook.
//
// Fonte única: o arquivo `configs.json` (modelo versionado em
// `configs_example.json`), com as chaves sensíveis podendo vir do ambiente
// (ver environment.go). A carga é feita por Environment, que aceita um caminho
// explícito (flag `--config` da CLI) ou procura o arquivo em `./configs.json` e
// `/etc/valheim-webhook/configs.json`, nessa ordem.
//
// Diferença deliberada em relação ao Atila: aqui a leitura é `encoding/json` da
// stdlib, não viper. O arquivo é JSON, o processo é um só e o que realmente
// precisa vir de fora do arquivo são três segredos do Discord — resolver isso
// com três `os.Getenv` explícitos é mais previsível do que a resolução
// implícita de env do viper, que silenciosamente ignora chave não registrada.
//
// Pacote folha: importa apenas a stdlib, e por isso pode ser usado por qualquer
// camada (infra, domain, application) sem violar as regras de dependência.
package config

import (
	"fmt"
	"strings"
	"time"
)

// Config espelha a estrutura do configs.json.
type Config struct {
	App        App        `json:"app"`
	Server     Server     `json:"server"`
	Security   Security   `json:"security"`
	Admin      Admin      `json:"admin"`
	Ingestao   Ingestao   `json:"ingestao"`
	Databases  Databases  `json:"databases"`
	Discord    Discord    `json:"discord"`
	Migrations Migrations `json:"migrations"`
	Painel     Painel     `json:"painel"`

	// arquivo guarda o caminho efetivamente carregado (diagnóstico e mensagens
	// de erro). Não vem do JSON.
	arquivo string
}

// App identifica a aplicação.
type App struct {
	Name    string `json:"name"`
	Env     string `json:"env"`
	Version string `json:"version"`
	// BaseDomain é o domínio guarda-chuva da instalação (ex.: `atila.cloud`).
	//
	// Ele resolve duas coisas de uma vez quando o painel deixa de ser
	// `localhost:6060` e passa a ser `valheim.atila.cloud`:
	//
	//   - CORS por SUFIXO: qualquer origem sob `.atila.cloud` é aceita, então
	//     um subdomínio novo (um segundo mundo, um painel interno) não exige
	//     mexer na configuração;
	//   - o DOMÍNIO do cookie de sessão, quando `security.cookie_dominio` não é
	//     preenchido — é o que permite a sessão valer entre subdomínios.
	//
	// Vazio (o padrão) mantém tudo restrito à origem exata: instalação em
	// `http://ip-da-lan:6060` não precisa de nada disto.
	BaseDomain string `json:"base_domain"`
}

// Server agrupa a configuração dos transportes (hoje só HTTP).
type Server struct {
	HTTP HTTP `json:"http"`
}

// HTTP configura o servidor Gin.
type HTTP struct {
	// Port é 6060 por padrão: é a porta que o hook do container de Valheim
	// chama em `http://host.docker.internal:6060/eventos`.
	Port         int      `json:"port"`
	TrustedProxy []string `json:"trusted_proxy"`
	CORS         CORS     `json:"cors"`
}

// CORS lista as origens permitidas.
//
// Vazio é o padrão e significa "nenhuma origem externa": o painel é servido
// pelo próprio processo, na mesma origem, e não precisa de CORS. A lista existe
// para quem for embutir o painel em outro lugar.
type CORS struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

// Security guarda o segredo de assinatura da sessão e a validade dela.
type Security struct {
	// JWTSecret assina o token da sessão. Vazio faz o boot GERAR um segredo
	// aleatório e avisar: o processo sobe e o painel funciona, mas todo mundo
	// é deslogado a cada reinício — o que é aceitável em dev e inaceitável em
	// produção (onde a validação exige o segredo preenchido).
	JWTSecret string `json:"jwt_secret"`
	// SessaoHoras é a validade do token. Padrão: 168h (uma semana), que é a
	// vida de uma aba de painel deixada aberta.
	SessaoHoras int `json:"sessao_horas"`
	// CookieSeguro marca a sessão como `Secure` (só trafega em HTTPS). Fica
	// desligado por padrão porque a instalação típica é `http://ip-da-lan:6060`
	// — ligar isso ali faria o navegador descartar o cookie e o login pareceria
	// "não funcionar". LIGUE ao publicar o painel num domínio com HTTPS.
	CookieSeguro bool `json:"cookie_seguro"`
	// CookieDominio é o `Domain` do cookie de sessão.
	//
	// Vazio (o padrão) faz o cookie valer só para o host que respondeu — que é
	// o correto e o mais restrito. Preencher com `.atila.cloud` faz a sessão
	// valer em todos os subdomínios, o que só interessa a quem tem mais de um.
	//
	// Quando vazio E `app.base_domain` está preenchido, o boot NÃO adivinha: a
	// escolha entre "só este host" e "todo o domínio" é de segurança, e
	// adivinhar erraria para o lado mais aberto.
	CookieDominio string `json:"cookie_dominio"`
}

// Admin é a conta de administração criada no primeiro boot.
//
// Ela existe porque um painel com gestão de usuários precisa de um primeiro
// usuário, e pedir para o dono do servidor rodar SQL à mão para tê-lo seria
// trocar um problema de produto por um problema de banco.
type Admin struct {
	Email string `json:"email"`
	// Senha vazia faz o boot SORTEAR uma e imprimi-la no log, uma vez. É o
	// caminho seguro por padrão: ninguém sobe o serviço com `admin/admin` sem
	// perceber.
	Senha string `json:"senha"`
	Nome  string `json:"nome"`
}

// Ingestao configura a porta de entrada dos eventos (`POST /eventos`).
type Ingestao struct {
	// Token, quando preenchido, é exigido no header `X-Ingest-Token`. Vazio
	// deixa a rota aberta — aceitável só quando o processo escuta em rede
	// privada, que é o caso do `host.docker.internal` da mesma máquina.
	Token string `json:"token"`
	// MaxBodyBytes é o teto do corpo aceito. Uma linha de log do Valheim tem
	// algumas centenas de bytes; o teto protege contra alguém despejar um
	// arquivo inteiro na rota.
	MaxBodyBytes int64 `json:"max_body_bytes"`
	// Servidor é o nome com que os eventos são carimbados (o `SERVER_NAME` do
	// container). Serve para o dia em que dois mundos apontarem para o mesmo
	// receptor.
	Servidor string `json:"servidor"`
}

// Databases reúne as dependências de dados. Só há uma, e ela é fatal.
type Databases struct {
	Postgres Postgres `json:"postgres"`
}

// Postgres é o banco transacional (obrigatório: sem ele a API não sobe).
type Postgres struct {
	Host    string `json:"host"`
	Port    string `json:"port"`
	User    string `json:"user"`
	Pwd     string `json:"pwd"`
	DBName  string `json:"db_name"`
	SSLMode string `json:"ssl_mode"`
	Pool    Pool   `json:"pool"`
}

// Pool são os limites do pool de conexões.
type Pool struct {
	MaxOpenConns       int `json:"max_open_conns"`
	MaxIdleConns       int `json:"max_idle_conns"`
	ConnMaxLifetimeMin int `json:"conn_max_lifetime_min"`
	ConnMaxIdleTimeMin int `json:"conn_max_idle_time_min"`
	HealthIntervalSec  int `json:"health_interval_sec"`
}

// Discord é o destino das notificações. Indisponível ou `enabled=false` → modo
// degradado: os eventos continuam sendo gravados e aparecendo no painel, só não
// saem para o chat.
type Discord struct {
	Enabled bool `json:"enabled"`
	// WebhookURL é o caminho mais simples (um webhook de canal, sem bot).
	WebhookURL string `json:"webhook_url"`
	// BotToken + ChannelID é o caminho do APP do Discord: a mensagem sai com a
	// identidade do bot e o mesmo token serve para o app crescer depois
	// (comandos, presença). Quando os dois estão preenchidos, este caminho
	// vence o webhook.
	BotToken  string `json:"bot_token"`
	ChannelID string `json:"channel_id"`
	// Username sobrescreve o nome exibido — só tem efeito no modo webhook.
	Username   string `json:"username"`
	TimeoutSec int    `json:"timeout_sec"`
	// Fila é o buffer de mensagens pendentes. Cheia, a mensagem é descartada
	// com log: notificar é efeito colateral, nunca segura a ingestão.
	Fila int `json:"fila"`
	// Eventos lista os tipos que merecem mensagem. Vazio = todos os tipos
	// conhecidos, menos os ruidosos (ver evento.TiposNotificaveisPadrao).
	Eventos []string `json:"eventos"`
}

// Migrations controla o runner do golang-migrate.
type Migrations struct {
	AutoRun             bool   `json:"auto_run"`
	Path                string `json:"path"`
	LockTimeoutSec      int    `json:"lock_timeout_sec"`
	StatementTimeoutMin int    `json:"statement_timeout_min"`
}

// Painel é o front-end servido pelo próprio binário.
type Painel struct {
	Enabled bool   `json:"enabled"`
	Titulo  string `json:"titulo"`
	// Dominio é o endereço público do painel (ex.: `valheim.atila.cloud`).
	//
	// O serviço não usa isto para ROTEAR nada — ele responde em qualquer Host,
	// e quem faz o roteamento é o proxy reverso na frente. Ele serve para
	// montar os endereços que o produto IMPRIME (o log do boot, as instruções
	// do painel) e para entrar na lista de origens aceitas pelo CORS.
	Dominio string `json:"dominio"`
	// EventosNoFeed é quantos eventos a tela carrega ao abrir; daí em diante o
	// fluxo é por SSE.
	EventosNoFeed int `json:"eventos_no_feed"`
}

// Arquivo devolve o caminho do configs.json efetivamente carregado.
func (c *Config) Arquivo() string { return c.arquivo }

// IsProduction indica ambiente produtivo (Gin em release, validações rígidas).
func (c *Config) IsProduction() bool {
	env := strings.ToLower(strings.TrimSpace(c.App.Env))
	return env == "prod" || env == "production"
}

// Addr é o endereço de escuta do servidor HTTP.
func (h HTTP) Addr() string { return fmt.Sprintf(":%d", h.Port) }

// SessaoExpiracao é a validade do token de sessão.
func (s Security) SessaoExpiracao() time.Duration {
	return time.Duration(s.SessaoHoras) * time.Hour
}

// SufixoDeOrigem devolve o sufixo aceito pelo CORS (`.atila.cloud`), ou ""
// quando não há domínio-base configurado.
//
// O ponto na frente não é enfeite: sem ele, `maliciosoatila.cloud` casaria com
// o sufixo `atila.cloud` e passaria pelo filtro.
func (a App) SufixoDeOrigem() string {
	base := strings.ToLower(strings.TrimSpace(a.BaseDomain))
	base = strings.TrimPrefix(base, ".")
	if base == "" {
		return ""
	}
	return "." + base
}

// URLPublica é o endereço do painel para exibição (log do boot, instruções).
//
// Sem domínio configurado, cai no endereço local — que é o que a pessoa
// realmente vai digitar numa instalação caseira.
func (c *Config) URLPublica() string {
	if dominio := strings.TrimSpace(c.Painel.Dominio); dominio != "" {
		return "https://" + dominio
	}
	return fmt.Sprintf("http://localhost:%d", c.Server.HTTP.Port)
}

// ConnMaxLifetime é o tempo de vida máximo de uma conexão do pool.
func (p Pool) ConnMaxLifetime() time.Duration {
	return time.Duration(p.ConnMaxLifetimeMin) * time.Minute
}

// ConnMaxIdleTime é o tempo máximo que uma conexão fica ociosa no pool.
func (p Pool) ConnMaxIdleTime() time.Duration {
	return time.Duration(p.ConnMaxIdleTimeMin) * time.Minute
}

// HealthInterval é o intervalo entre os pings do health check do pool.
func (p Pool) HealthInterval() time.Duration {
	return time.Duration(p.HealthIntervalSec) * time.Second
}

// LockTimeout é o tempo máximo que a conexão de migração espera por um lock.
func (m Migrations) LockTimeout() time.Duration {
	return time.Duration(m.LockTimeoutSec) * time.Second
}

// StatementTimeout é o tempo máximo de execução de uma migration.
func (m Migrations) StatementTimeout() time.Duration {
	return time.Duration(m.StatementTimeoutMin) * time.Minute
}

// Timeout é o prazo de uma chamada ao Discord.
func (d Discord) Timeout() time.Duration {
	return time.Duration(d.TimeoutSec) * time.Second
}

// ModoBot diz se as mensagens saem pela API do app (bot) em vez do webhook.
func (d Discord) ModoBot() bool {
	return strings.TrimSpace(d.BotToken) != "" && strings.TrimSpace(d.ChannelID) != ""
}

// Configurado diz se há para onde mandar mensagem.
func (d Discord) Configurado() bool {
	return d.ModoBot() || strings.TrimSpace(d.WebhookURL) != ""
}

// Ativo junta as duas perguntas: ligado no arquivo E com destino definido.
func (d Discord) Ativo() bool { return d.Enabled && d.Configurado() }
