// Package memoria mantém as últimas linhas do log do processo em memória, para
// que o painel possa mostrá-las sem que ninguém precise abrir um terminal.
//
// A estrutura é um anel de tamanho fixo: quando enche, a linha mais antiga é
// sobrescrita. É a escolha certa para o que isto é — uma janela de diagnóstico,
// não um arquivo de log. O log de verdade continua saindo no stdout do
// processo, que é o que o `docker logs` e o journald recolhem.
//
// Nada aqui vai para o banco. Um receptor de eventos que gravasse o próprio log
// no mesmo Postgres que ele usa para funcionar teria um problema divertido no
// dia em que o Postgres caísse.
//
// Pacote folha: importa apenas a stdlib.
package memoria

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// CapacidadePadrao é quantas linhas o anel guarda.
//
// Quinhentas linhas cobrem o boot inteiro mais um bom tempo de operação, e
// custam poucas centenas de KB — a ordem de grandeza certa para algo que existe
// só para responder "o que aconteceu agora há pouco?".
const CapacidadePadrao = 500

// Linha é uma entrada do log, já achatada para exibição.
type Linha struct {
	// Seq é crescente e nunca se repete: é o que o painel usa para pedir "o que
	// houve depois da última que eu vi".
	Seq       int64             `json:"seq"`
	Ts        time.Time         `json:"ts"`
	Nivel     string            `json:"nivel"`
	Mensagem  string            `json:"mensagem"`
	Atributos map[string]string `json:"atributos,omitempty"`
}

// Buffer é o anel de linhas.
type Buffer struct {
	mu         sync.RWMutex
	linhas     []Linha
	proxima    int
	cheio      bool
	capacidade int
	seq        int64
}

// NovoBuffer cria o anel. Capacidade menor que 1 cai em CapacidadePadrao.
func NovoBuffer(capacidade int) *Buffer {
	if capacidade < 1 {
		capacidade = CapacidadePadrao
	}
	return &Buffer{linhas: make([]Linha, capacidade), capacidade: capacidade}
}

// Anexar grava uma linha, sobrescrevendo a mais antiga quando cheio.
func (b *Buffer) Anexar(l Linha) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.seq++
	l.Seq = b.seq
	b.linhas[b.proxima] = l
	b.proxima = (b.proxima + 1) % b.capacidade
	if b.proxima == 0 {
		b.cheio = true
	}
}

// Filtro é o recorte pedido pelo painel.
type Filtro struct {
	// Nivel mínimo exibido ("", "DEBUG", "INFO", "WARN", "ERROR").
	Nivel string
	// Busca é um trecho procurado na mensagem e nos atributos.
	Busca string
	// DepoisDe devolve só o que veio depois desta sequência (polling do painel).
	DepoisDe int64
	// Limite é o teto de linhas devolvidas (0 = tudo o que o anel tem).
	Limite int
}

// Ultimas devolve as linhas que casam com o filtro, da mais ANTIGA para a mais
// recente — a ordem em que se lê um log.
func (b *Buffer) Ultimas(f Filtro) []Linha {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()

	minimo, temMinimo := nivelDe(f.Nivel)
	busca := strings.ToLower(strings.TrimSpace(f.Busca))

	// Percorre em ordem cronológica: do mais antigo vivo até o mais recente.
	total := b.capacidade
	inicio := b.proxima
	if !b.cheio {
		total, inicio = b.proxima, 0
	}

	escolhidas := make([]Linha, 0, total)
	for i := 0; i < total; i++ {
		linha := b.linhas[(inicio+i)%b.capacidade]
		if linha.Seq == 0 || linha.Seq <= f.DepoisDe {
			continue
		}
		if temMinimo {
			if nivel, ok := nivelDe(linha.Nivel); ok && nivel < minimo {
				continue
			}
		}
		if busca != "" && !casa(linha, busca) {
			continue
		}
		escolhidas = append(escolhidas, linha)
	}

	// O corte é pelo FIM: com mais linhas do que o limite, o que interessa é o
	// que acabou de acontecer.
	if f.Limite > 0 && len(escolhidas) > f.Limite {
		escolhidas = escolhidas[len(escolhidas)-f.Limite:]
	}
	return escolhidas
}

// UltimaSequencia devolve o número da linha mais recente.
func (b *Buffer) UltimaSequencia() int64 {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.seq
}

// casa procura o trecho na mensagem e nos atributos.
func casa(l Linha, busca string) bool {
	if strings.Contains(strings.ToLower(l.Mensagem), busca) {
		return true
	}
	for chave, valor := range l.Atributos {
		if strings.Contains(strings.ToLower(chave), busca) ||
			strings.Contains(strings.ToLower(valor), busca) {
			return true
		}
	}
	return false
}

// nivelDe traduz o texto do nível no valor do slog.
func nivelDe(nome string) (slog.Level, bool) {
	switch strings.ToUpper(strings.TrimSpace(nome)) {
	case "DEBUG":
		return slog.LevelDebug, true
	case "INFO":
		return slog.LevelInfo, true
	case "WARN", "WARNING":
		return slog.LevelWarn, true
	case "ERROR", "ERRO":
		return slog.LevelError, true
	default:
		return 0, false
	}
}
