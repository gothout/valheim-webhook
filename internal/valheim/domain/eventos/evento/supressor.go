package evento

import (
	"sync"
	"time"
)

// JanelaDeSaida é por quanto tempo uma saída já registrada suprime as
// repetições do mesmo jogador.
//
// O número sai do log real deste servidor: quando alguém desconecta, o Valheim
// escreve uma linha `Destroying abandoned non persistent zdo <dono>:<n>` para
// CADA objeto que a pessoa deixou no mundo — no exemplo observado, 40+ linhas
// no MESMO segundo, todas com o mesmo dono. Sem supressão, uma saída viraria 40
// eventos idênticos no feed e 40 escritas no banco.
//
// Trinta segundos é folgado para cobrir a limpeza inteira (que leva menos de um
// segundo) e curto para não engolir uma saída de verdade: ninguém sai, reentra
// e sai de novo em meio minuto.
const JanelaDeSaida = 30 * time.Second

// supressorDeSaidas lembra qual dono já teve a saída registrada há pouco.
//
// Ele vive em MEMÓRIA, e não no banco, de propósito. A alternativa seria uma
// consulta por linha recebida (40 por saída) para responder algo que só
// interessa por trinta segundos. O custo de perdê-lo num reinício é conhecido e
// pequeno: no máximo uma saída duplicada, se o processo reiniciar exatamente no
// meio de uma limpeza.
type supressorDeSaidas struct {
	mu      sync.Mutex
	ultimas map[string]time.Time
	janela  time.Duration
}

// novoSupressor cria o supressor com a janela informada.
func novoSupressor(janela time.Duration) *supressorDeSaidas {
	if janela <= 0 {
		janela = JanelaDeSaida
	}
	return &supressorDeSaidas{ultimas: make(map[string]time.Time), janela: janela}
}

// Repetida diz se a saída deste dono já foi registrada dentro da janela e, se
// não foi, MARCA que agora foi.
//
// As duas coisas num método só, sob a mesma trava: separar "consultar" de
// "marcar" abriria uma janela entre elas, e duas linhas da mesma enxurrada
// chegando em paralelo passariam as duas.
//
// Dono vazio nunca é suprimido: sem identificação, não há como afirmar que é a
// mesma saída.
func (s *supressorDeSaidas) Repetida(dono string, agora time.Time) bool {
	if s == nil || dono == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if ultima, visto := s.ultimas[dono]; visto && agora.Sub(ultima) < s.janela {
		return true
	}

	s.limpar(agora)
	s.ultimas[dono] = agora
	return false
}

// Esquecer apaga a marca de um dono — chamado quando o jogador entra de novo,
// para a próxima saída dele contar mesmo dentro da janela.
func (s *supressorDeSaidas) Esquecer(dono string) {
	if s == nil || dono == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.ultimas, dono)
}

// limpar descarta as marcas vencidas. Roda junto da escrita, que é rara (uma
// por saída de verdade) — o mapa nunca passa de alguns jogadores.
//
// Precisa ser chamado com a trava em mãos.
func (s *supressorDeSaidas) limpar(agora time.Time) {
	for dono, quando := range s.ultimas {
		if agora.Sub(quando) >= s.janela {
			delete(s.ultimas, dono)
		}
	}
}
