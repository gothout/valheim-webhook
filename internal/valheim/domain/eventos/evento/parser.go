package evento

import (
	"regexp"
	"strings"
	"time"
)

// Analise é o resultado da leitura de uma linha de log: os campos que o parser
// conseguiu extrair, sem nada de persistência.
//
// Função pura e testável — é o coração do subdomínio e o arquivo com mais
// testes, porque é o único lugar onde uma atualização do jogo pode quebrar o
// produto em silêncio.
type Analise struct {
	Tipo    Tipo
	Jogador string
	SteamID string
	ZDOID   string
	Texto   string
	// OcorridoEm é a hora carimbada na linha, em UTC. Zero quando a linha não
	// traz carimbo.
	OcorridoEm time.Time
	// TemHora distingue "sem carimbo" de "carimbo à meia-noite".
	TemHora bool
}

// Padrões das linhas que o servidor de Valheim emite.
//
// Todos são deliberadamente FROUXOS no começo e no fim: a linha chega com
// carimbo de hora na frente e, dependendo da versão, com sufixo depois. Prender
// o regex às pontas seria trocar "não reconheci este campo" por "não reconheci
// esta linha" a cada atualização do jogo.
var (
	// `Got character ZDOID from Fulano de Tal : -1146266487:1`
	// O nome do personagem aceita espaço e acento — daí o `(.+?)` preguiçoso
	// até o ` : ` que separa o nome do ZDOID.
	padraoPersonagem = regexp.MustCompile(`Got character ZDOID from\s+(.+?)\s*:\s*(-?\d+):(-?\d+)`)

	// `Got connection SteamID 76561198005507113`
	padraoConexao = regexp.MustCompile(`Got connection SteamID\s+(\d+)`)

	// `Destroying abandoned non persistent zdo -1146266487:1 own 0`
	// O hífen de `non-persistent` aparece em algumas versões e não em outras.
	padraoAbandono = regexp.MustCompile(`Destroying abandoned non[- ]persistent zdo\s+(-?\d+):(-?\d+)`)

	// `Game server connected`
	padraoServidor = regexp.MustCompile(`Game server connected`)

	// As linhas `Got text`. O formato varia entre versões do jogo, então há
	// três tentativas, da mais específica para a mais frouxa, e a última
	// preserva tudo o que veio depois de `Got text`. Nenhuma delas descarta a
	// linha: pior caso, o texto sai inteiro no campo `texto`.
	padraoTextoDe    = regexp.MustCompile(`Got text\s*["']?(.*?)["']?\s+from\s+(.+?)\s*$`)
	padraoTextoDePre = regexp.MustCompile(`Got text\s+from\s+(.+?)\s*:\s*(.+?)\s*$`)
	padraoTexto      = regexp.MustCompile(`Got text\s*:?\s*(.+?)\s*$`)

	// Carimbo de hora no começo da linha: `03/24/2024 22:31:07: `.
	padraoCarimbo = regexp.MustCompile(`^(\d{2}/\d{2}/\d{4} \d{2}:\d{2}:\d{2})\s*:\s*`)
	// Formato ISO, emitido por algumas configurações de locale do container.
	padraoCarimboISO = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2})\s*:?\s*`)
)

// ZDOIDMorte é o ZDOID que o servidor emite quando o personagem morre: o objeto
// do personagem deixou de existir no mundo.
const ZDOIDMorte = "0:0"

// Analisar interpreta uma linha de log.
//
// Nunca devolve erro por "não entendi": linha desconhecida vira
// `TipoDesconhecido` com a linha preservada. O único erro possível é a linha
// vazia, que não é log de nada.
func Analisar(linha string) (Analise, error) {
	limpa := strings.TrimSpace(linha)
	if limpa == "" {
		return Analise{}, ErrLinhaVazia
	}

	a := Analise{Tipo: TipoDesconhecido}
	corpo := limpa

	if quando, resto, achou := extrairCarimbo(limpa); achou {
		a.OcorridoEm, a.TemHora, corpo = quando, true, resto
	}

	switch {
	case padraoPersonagem.MatchString(corpo):
		partes := padraoPersonagem.FindStringSubmatch(corpo)
		a.Jogador = strings.TrimSpace(partes[1])
		a.ZDOID = partes[2] + ":" + partes[3]
		if a.ZDOID == ZDOIDMorte {
			a.Tipo = TipoMorreu
			// O ZDOID zerado não identifica objeto nenhum — guardá-lo faria a
			// busca por ZDOID do evento de saída casar com toda morte do
			// servidor.
			a.ZDOID = ""
		} else {
			a.Tipo = TipoEntrou
		}

	case padraoConexao.MatchString(corpo):
		a.Tipo = TipoConexao
		a.SteamID = padraoConexao.FindStringSubmatch(corpo)[1]

	case padraoAbandono.MatchString(corpo):
		partes := padraoAbandono.FindStringSubmatch(corpo)
		a.Tipo = TipoSaiu
		a.ZDOID = partes[1] + ":" + partes[2]

	case padraoServidor.MatchString(corpo):
		a.Tipo = TipoServidorPronto

	case strings.Contains(corpo, "Got text"):
		a.Tipo = TipoMensagem
		a.Jogador, a.Texto = extrairMensagem(corpo)
	}

	return a, nil
}

// extrairCarimbo tira a hora da frente da linha e devolve o resto.
//
// O formato do Valheim é MM/DD/AAAA (o servidor é um binário .NET com cultura
// invariante). A tentativa em DD/MM/AAAA existe como rede de segurança para
// container com locale diferente: `12/03` é ambíguo, mas `24/03` só é válido
// numa das duas leituras, e é isso que o segundo layout resolve.
//
// A hora vem SEM fuso — é a hora local da máquina do servidor de Valheim. Ela é
// interpretada no fuso local deste processo (que roda na mesma máquina, ou com
// o mesmo TZ) e convertida para UTC.
func extrairCarimbo(linha string) (time.Time, string, bool) {
	if partes := padraoCarimbo.FindStringSubmatch(linha); partes != nil {
		resto := strings.TrimSpace(linha[len(partes[0]):])
		for _, layout := range []string{"01/02/2006 15:04:05", "02/01/2006 15:04:05"} {
			if quando, err := time.ParseInLocation(layout, partes[1], time.Local); err == nil {
				return quando.UTC(), resto, true
			}
		}
		return time.Time{}, resto, false
	}

	if partes := padraoCarimboISO.FindStringSubmatch(linha); partes != nil {
		resto := strings.TrimSpace(linha[len(partes[0]):])
		normalizada := strings.Replace(partes[1], "T", " ", 1)
		if quando, err := time.ParseInLocation("2006-01-02 15:04:05", normalizada, time.Local); err == nil {
			return quando.UTC(), resto, true
		}
		return time.Time{}, resto, false
	}

	return time.Time{}, linha, false
}

// extrairMensagem separa o autor do conteúdo numa linha `Got text`.
//
// Devolve (autor, texto). Autor vazio quando a linha não o nomeia — o que é
// melhor do que chutar: uma placa colocada no mundo também vira `Got text`, e
// atribuí-la a quem passou por perto seria inventar.
func extrairMensagem(corpo string) (string, string) {
	if partes := padraoTextoDePre.FindStringSubmatch(corpo); partes != nil {
		return strings.TrimSpace(partes[1]), limparAspas(partes[2])
	}
	if partes := padraoTextoDe.FindStringSubmatch(corpo); partes != nil {
		return strings.TrimSpace(partes[2]), limparAspas(partes[1])
	}
	if partes := padraoTexto.FindStringSubmatch(corpo); partes != nil {
		return "", limparAspas(partes[1])
	}
	return "", ""
}

// limparAspas tira aspas simples ou duplas que envolvam o texto inteiro.
func limparAspas(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		primeira, ultima := s[0], s[len(s)-1]
		if (primeira == '"' && ultima == '"') || (primeira == '\'' && ultima == '\'') {
			return strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}
