// Package discord publica as mensagens do servidor de Valheim no Discord.
//
// Dois modos, escolhidos pela configuração:
//
//   - APP/BOT (`discord.bot_token` + `discord.channel_id`): a mensagem sai pela
//     API do app, com a identidade do bot. É o caminho que cresce — o mesmo
//     token serve depois para comandos e presença;
//   - WEBHOOK (`discord.webhook_url`): um webhook de canal, sem app nenhum. É o
//     caminho de dois minutos, para quem só quer ver o evento aparecer.
//
// Com os dois preenchidos, o modo bot vence.
//
// Esta dependência é DEGRADÁVEL: sem destino configurado, `Connect` devolve
// cliente nulo com log `[DEGRADADO]` e o consumidor é obrigado a tratar a
// ausência. Um receptor de eventos que não sobe porque o token do Discord está
// errado seria um receptor que perde eventos por causa de um chat.
//
// O envio é ASSÍNCRONO: `Enviar` só enfileira. Nenhuma linha de log do Valheim
// pode ficar esperando o Discord responder — se a API estiver lenta, o servidor
// de jogo continua despejando eventos.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"valheim-webhook/internal/pkg/config"
)

// BaseAPI é a raiz da API do Discord usada no modo bot.
const BaseAPI = "https://discord.com/api/v10"

// TentativasPorMensagem é quantas vezes uma mensagem é tentada.
//
// Duas: a original e uma repetição. O que se quer cobrir é o 429 (limite de
// taxa) e a falha de rede pontual. Insistir mais do que isso, numa fila que
// continua enchendo, só atrasa os eventos seguintes.
const TentativasPorMensagem = 2

// EsperaMaximaDoLimite é o teto da espera pedida por um 429. Acima disso, a
// mensagem é descartada: um evento de jogo perde o valor em minutos.
const EsperaMaximaDoLimite = 30 * time.Second

// Cliente é o publicador do processo.
//
// Todos os métodos são seguros com receptor nulo — é assim que o modo degradado
// funciona sem espalhar `if cliente != nil` pelo código de quem chama.
type Cliente struct {
	http     *http.Client
	modoBot  bool
	destino  string
	token    string
	username string

	fila  chan Mensagem
	parar chan struct{}
	fim   chan struct{}

	fechado     atomic.Bool
	enviadas    atomic.Int64
	falhas      atomic.Int64
	descartadas atomic.Int64
}

// Connect monta o cliente e sobe o trabalhador de envio.
//
// Nunca devolve erro: destino ausente ou `enabled=false` é modo degradado, não
// falha de boot.
func Connect(cfg config.Discord) *Cliente {
	if !cfg.Enabled {
		slog.Warn("[DISCORD] [DEGRADADO] desligado na configuração (discord.enabled=false): " +
			"os eventos seguem sendo gravados e exibidos no painel")
		return nil
	}
	if !cfg.Configurado() {
		slog.Warn("[DISCORD] [DEGRADADO] sem destino: preencha discord.webhook_url " +
			"ou discord.bot_token + discord.channel_id")
		return nil
	}

	c := &Cliente{
		http:     &http.Client{Timeout: cfg.Timeout()},
		modoBot:  cfg.ModoBot(),
		username: strings.TrimSpace(cfg.Username),
		fila:     make(chan Mensagem, cfg.Fila),
		parar:    make(chan struct{}),
		fim:      make(chan struct{}),
	}

	if c.modoBot {
		c.destino = fmt.Sprintf("%s/channels/%s/messages", BaseAPI, strings.TrimSpace(cfg.ChannelID))
		c.token = strings.TrimSpace(cfg.BotToken)
		// No modo bot o nome exibido é o do próprio app — o campo `username` do
		// corpo é ignorado pela API e mandá-lo só polui o payload.
		c.username = ""
		slog.Info("[DISCORD] conectado (app/bot)", "canal", cfg.ChannelID)
	} else {
		c.destino = strings.TrimSpace(cfg.WebhookURL)
		slog.Info("[DISCORD] conectado (webhook)", "username", c.username)
	}

	go c.trabalhar()
	return c
}

// Enviar enfileira a mensagem. Nunca bloqueia e nunca devolve erro.
//
// Fila cheia = mensagem descartada e contada. É a escolha certa aqui: com o
// Discord fora do ar, o que não pode acontecer é a ingestão parar.
func (c *Cliente) Enviar(m Mensagem) {
	if c == nil || c.fechado.Load() {
		return
	}
	select {
	case c.fila <- m:
	default:
		if descartadas := c.descartadas.Add(1); descartadas%50 == 1 {
			slog.Warn("[DISCORD] fila cheia: mensagens descartadas",
				"descartadas", descartadas, "capacidade", cap(c.fila))
		}
	}
}

// Disponivel diz se há para onde mandar mensagem.
func (c *Cliente) Disponivel() bool { return c != nil && !c.fechado.Load() }

// Metricas devolve os contadores do publicador (health check e painel).
func (c *Cliente) Metricas() (enviadas, falhas, descartadas int64) {
	if c == nil {
		return 0, 0, 0
	}
	return c.enviadas.Load(), c.falhas.Load(), c.descartadas.Load()
}

// Close drena o que está na fila e encerra o trabalhador. Idempotente.
func (c *Cliente) Close() error {
	if c == nil || c.fechado.Swap(true) {
		return nil
	}
	close(c.parar)
	<-c.fim
	return nil
}

// trabalhar é o laço de envio: uma mensagem por vez, em ordem.
//
// Serial de propósito. O limite de taxa do Discord é por canal, e mandar em
// paralelo só antecipa o 429 — além de embaralhar a ordem dos eventos, que é
// justamente o que conta a história da partida.
func (c *Cliente) trabalhar() {
	defer close(c.fim)
	for {
		select {
		case m := <-c.fila:
			c.entregar(m)
		case <-c.parar:
			// Drena o que já estava na fila antes de sair: o último "fulano
			// saiu" do shutdown ainda vale a pena.
			for {
				select {
				case m := <-c.fila:
					c.entregar(m)
				default:
					return
				}
			}
		}
	}
}

// EsperaEntreTentativas é a pausa antes de repetir uma falha que não pediu
// espera explícita (queda de rede, 5xx do Discord).
const EsperaEntreTentativas = 500 * time.Millisecond

// entregar faz o POST, com uma repetição em caso de 429 ou falha passageira.
//
// Duas recusas NÃO são repetidas, porque repetir não muda o resultado:
// credencial recusada (401/403) e limite de taxa com espera acima do teto.
func (c *Cliente) entregar(m Mensagem) {
	dados, err := json.Marshal(paraCorpo(m, c.username))
	if err != nil {
		c.falhas.Add(1)
		slog.Error("[DISCORD] mensagem não serializável", "erro", err)
		return
	}

	for tentativa := 1; ; tentativa++ {
		espera, err := c.postar(dados)
		if err == nil {
			c.enviadas.Add(1)
			return
		}

		definitivo := errors.Is(err, ErrCredencial) || errors.Is(err, ErrLimiteExcedido)
		if definitivo || tentativa >= TentativasPorMensagem {
			c.falhas.Add(1)
			slog.Error("[DISCORD] falha ao publicar", "tentativa", tentativa, "erro", err)
			return
		}

		if espera <= 0 {
			espera = EsperaEntreTentativas
		}
		slog.Warn("[DISCORD] repetindo o envio", "espera", espera, "erro", err)

		select {
		case <-time.After(espera):
		case <-c.parar:
			// Encerrando: não vale segurar o shutdown por uma mensagem.
			c.falhas.Add(1)
			slog.Error("[DISCORD] encerrando com mensagem não publicada", "erro", err)
			return
		}
	}
}

// postar executa uma tentativa.
//
// Devolve a espera pedida pelo servidor quando a recusa é 429 (limite de taxa);
// zero em qualquer outro caso.
func (c *Cliente) postar(dados []byte) (time.Duration, error) {
	ctx, cancelar := context.WithTimeout(context.Background(), c.http.Timeout)
	defer cancelar()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.destino, bytes.NewReader(dados))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "valheim-webhook (+https://github.com/community-valheim-tools/valheim-server-docker)")
	if c.modoBot {
		req.Header.Set("Authorization", "Bot "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	// O corpo precisa ser drenado para a conexão voltar ao pool; 4 KiB é mais do
	// que qualquer erro da API do Discord ocupa.
	corpoResposta, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 0, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		espera := esperaDoLimite(resp)
		if espera <= 0 {
			return 0, fmt.Errorf("%w: o Discord pediu mais de %s de espera", ErrLimiteExcedido, EsperaMaximaDoLimite)
		}
		return espera, fmt.Errorf("%w: limite de taxa (HTTP 429)", ErrEnvio)
	}
	// 401/403 é token ou permissão errada, e vai continuar errado na próxima
	// mensagem: a mensagem de erro precisa dizer isso por extenso, porque é o
	// erro que o dono do servidor vai ver no log e ter de consertar sozinho.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return 0, fmt.Errorf("%w (HTTP %d): confira o token do bot e a permissão de escrever no canal: %s",
			ErrCredencial, resp.StatusCode, strings.TrimSpace(string(corpoResposta)))
	}
	return 0, fmt.Errorf("%w: HTTP %d: %s", ErrEnvio, resp.StatusCode, strings.TrimSpace(string(corpoResposta)))
}

// esperaDoLimite lê o `Retry-After` da resposta 429, dentro do teto.
func esperaDoLimite(resp *http.Response) time.Duration {
	segundos, err := strconv.ParseFloat(strings.TrimSpace(resp.Header.Get("Retry-After")), 64)
	if err != nil || segundos <= 0 {
		return time.Second
	}
	espera := time.Duration(segundos * float64(time.Second))
	if espera > EsperaMaximaDoLimite {
		return 0 // acima do teto: não vale a pena esperar
	}
	return espera
}
