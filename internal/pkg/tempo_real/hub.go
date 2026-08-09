// Package tempo_real é o barramento de publicação do painel: o que a ingestão
// acabou de gravar aparece na tela de quem está com o navegador aberto, sem
// recarregar e sem polling.
//
// O transporte é Server-Sent Events (SSE), e não WebSocket, por uma razão só: o
// tráfego aqui é de mão única — o servidor conta o que aconteceu, o navegador
// escuta. SSE resolve isso com uma requisição HTTP comum, reconecta sozinho no
// navegador e não precisa de biblioteca nenhuma dos dois lados.
//
// Pacote folha de `internal/pkg`: importa gin e stdlib. Quem publica é a camada
// de aplicação, através de uma interface declarada lá (a `Publicador` do
// `ingestao`) — este pacote não conhece nenhum domínio.
package tempo_real

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// CapacidadePadrao é o buffer de cada assinante.
//
// Ele existe para absorver rajada (uma partida inteira entrando ao mesmo tempo)
// sem segurar quem publica. Cheio, a mensagem MAIS ANTIGA daquele assinante é
// descartada — o navegador atrasado perde o começo da rajada, e não o fim, que
// é o que ele quer ver. Ninguém trava por causa de uma aba esquecida aberta.
const CapacidadePadrao = 32

// Mensagem é o que trafega no barramento: o nome do evento SSE e o corpo já
// serializado em JSON.
//
// A serialização acontece UMA vez, no Publicar, e não uma vez por assinante:
// com dez abas abertas, o custo é o mesmo de uma.
type Mensagem struct {
	Nome  string
	Dados []byte
}

// Assinante é uma conexão aberta do painel.
type Assinante struct {
	canal chan Mensagem
}

// Canal expõe a fila de leitura do assinante (usada pelo handler SSE).
func (a *Assinante) Canal() <-chan Mensagem { return a.canal }

// Hub distribui as mensagens para os assinantes conectados.
//
// O valor zero não é utilizável — construir sempre com NovoHub.
type Hub struct {
	mu         sync.RWMutex
	assinantes map[*Assinante]struct{}
	capacidade int
	descartes  int64
}

// NovoHub cria o barramento. Capacidade menor que 1 cai em CapacidadePadrao.
func NovoHub(capacidade int) *Hub {
	if capacidade < 1 {
		capacidade = CapacidadePadrao
	}
	return &Hub{
		assinantes: make(map[*Assinante]struct{}),
		capacidade: capacidade,
	}
}

// Assinar registra uma conexão nova e devolve o assinante.
func (h *Hub) Assinar() *Assinante {
	a := &Assinante{canal: make(chan Mensagem, h.capacidade)}
	h.mu.Lock()
	h.assinantes[a] = struct{}{}
	h.mu.Unlock()
	return a
}

// Cancelar remove a conexão e fecha o canal. Idempotente: o handler chama no
// `defer` e o encerramento do processo pode chamar de novo.
func (h *Hub) Cancelar(a *Assinante) {
	if a == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, existe := h.assinantes[a]; !existe {
		return
	}
	delete(h.assinantes, a)
	close(a.canal)
}

// Publicar serializa o corpo e entrega a todos os assinantes.
//
// Nunca bloqueia e nunca devolve erro: publicar no painel é efeito colateral da
// ingestão, não parte dela. Corpo que não serializa vira uma linha de log e o
// evento segue seu caminho — o Discord e o banco não podem perder um evento
// porque a tela de alguém teria problema para exibi-lo.
func (h *Hub) Publicar(nome string, corpo any) {
	dados, err := json.Marshal(corpo)
	if err != nil {
		slog.Error("[TEMPO-REAL] corpo não serializável; mensagem descartada",
			"evento", nome, "erro", err)
		return
	}

	msg := Mensagem{Nome: nome, Dados: dados}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for a := range h.assinantes {
		enviar(a, msg)
	}
}

// enviar entrega ao assinante descartando o mais antigo quando o buffer está
// cheio. O descarte é do canal DELE — nenhum outro assinante é afetado.
func enviar(a *Assinante, msg Mensagem) {
	select {
	case a.canal <- msg:
		return
	default:
	}
	// Buffer cheio: abre espaço jogando fora a mensagem mais antiga. O segundo
	// `default` cobre a corrida com o handler, que pode ter lido no meio.
	select {
	case <-a.canal:
	default:
	}
	select {
	case a.canal <- msg:
	default:
	}
}

// Assinantes informa quantas conexões estão abertas (health check e painel).
func (h *Hub) Assinantes() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.assinantes)
}

// Fechar derruba todas as conexões. Chamado no encerramento do processo, antes
// de o servidor HTTP parar de aceitar requests.
func (h *Hub) Fechar() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for a := range h.assinantes {
		delete(h.assinantes, a)
		close(a.canal)
	}
}
