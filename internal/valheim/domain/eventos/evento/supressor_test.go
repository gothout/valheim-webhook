package evento

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestSupressorEngoleAEnxurradaDeUmaSaidaSo reproduz o que o servidor de
// verdade faz: ao desconectar, ele emite uma linha `Destroying abandoned…` por
// objeto que a pessoa deixou no mundo — dezenas, no mesmo segundo, todas com o
// mesmo dono.
func TestSupressorEngoleAEnxurradaDeUmaSaidaSo(t *testing.T) {
	s := novoSupressor(JanelaDeSaida)
	agora := time.Date(2026, 8, 9, 5, 20, 27, 0, time.UTC)

	assert.False(t, s.Repetida("-53465420", agora), "a primeira linha é a saída")

	for i := 1; i <= 40; i++ {
		instante := agora.Add(time.Duration(i) * time.Millisecond)
		assert.True(t, s.Repetida("-53465420", instante), "linha %d é repetição", i)
	}
}

// TestSupressorNaoConfundeJogadoresDiferentes — duas pessoas saindo juntas são
// duas saídas.
func TestSupressorNaoConfundeJogadoresDiferentes(t *testing.T) {
	s := novoSupressor(JanelaDeSaida)
	agora := time.Now().UTC()

	assert.False(t, s.Repetida("-53465420", agora))
	assert.False(t, s.Repetida("1814428521", agora), "outro dono, outra saída")
	assert.True(t, s.Repetida("-53465420", agora.Add(time.Second)))
}

// TestSupressorLiberaDepoisDaJanela — passada a janela, uma saída nova conta.
func TestSupressorLiberaDepoisDaJanela(t *testing.T) {
	s := novoSupressor(JanelaDeSaida)
	agora := time.Now().UTC()

	assert.False(t, s.Repetida("-53465420", agora))
	assert.False(t, s.Repetida("-53465420", agora.Add(JanelaDeSaida+time.Second)))
}

// TestSupressorEsquecerLiberaQuemVoltou é a razão de `Esquecer` existir: quem
// sai e volta dentro da janela precisa ter a PRÓXIMA saída registrada.
func TestSupressorEsquecerLiberaQuemVoltou(t *testing.T) {
	s := novoSupressor(JanelaDeSaida)
	agora := time.Now().UTC()

	assert.False(t, s.Repetida("-53465420", agora))
	s.Esquecer("-53465420") // entrou de novo
	assert.False(t, s.Repetida("-53465420", agora.Add(2*time.Second)),
		"depois de reentrar, a saída seguinte conta")
}

// TestSupressorSemDonoNaoSuprime — sem identificação, não dá para afirmar que é
// a mesma saída, e suprimir seria engolir eventos de pessoas diferentes.
func TestSupressorSemDonoNaoSuprime(t *testing.T) {
	s := novoSupressor(JanelaDeSaida)
	agora := time.Now().UTC()

	assert.False(t, s.Repetida("", agora))
	assert.False(t, s.Repetida("", agora))
}
