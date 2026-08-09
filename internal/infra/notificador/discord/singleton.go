package discord

import (
	"log/slog"
	"sync"

	"valheim-webhook/internal/pkg/config"
)

var (
	mu        sync.RWMutex
	instancia *Cliente
	iniciado  bool
)

// InitDiscord monta o publicador do processo uma única vez, no boot.
//
// Não devolve erro de propósito: esta dependência é degradável (ver o
// comentário do pacote). Sem destino configurado, o publicador fica nulo e
// todos os métodos continuam seguros de chamar.
func InitDiscord(cfg config.Discord) *Cliente {
	mu.Lock()
	defer mu.Unlock()
	if iniciado {
		return instancia
	}
	iniciado = true
	instancia = Connect(cfg)
	return instancia
}

// Reconfigurar troca o publicador em tempo de execução.
//
// É o que faz a tela de integração do painel valer NA HORA: quem acabou de
// colar o token do bot não deveria precisar reiniciar o serviço para ver a
// primeira mensagem chegar.
//
// A troca é atômica do ponto de vista de quem publica (`Use()` devolve ou o
// antigo, ou o novo, nunca um estado quebrado), e o publicador antigo é
// encerrado EM SEGUNDO PLANO: o `Close` drena a fila dele, o que pode custar
// segundos se o Discord estiver lento, e a resposta do painel não tem por que
// esperar por isso.
func Reconfigurar(cfg config.Discord) *Cliente {
	novo := Connect(cfg)

	mu.Lock()
	antigo := instancia
	instancia = novo
	iniciado = true
	mu.Unlock()

	if antigo != nil {
		go func() {
			if err := antigo.Close(); err != nil {
				slog.Warn("[DISCORD] falha ao encerrar o publicador anterior", "erro", err)
			}
		}()
	}
	return novo
}

// Use devolve o publicador do processo — possivelmente nulo (modo degradado).
// Quem chama não precisa conferir: os métodos aceitam receptor nulo.
func Use() *Cliente {
	mu.RLock()
	defer mu.RUnlock()
	return instancia
}

// Disponivel diz se o publicador está de pé (health check da API).
func Disponivel() bool { return Use().Disponivel() }

// Close drena a fila e encerra o trabalhador. No-op sem publicador.
func Close() error { return Use().Close() }
