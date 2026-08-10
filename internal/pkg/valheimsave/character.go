package valheimsave

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

// Character é o save de um jogador (`.fch`).
//
// Diferente do mundo, este arquivo vive na máquina de cada pessoa (Steam
// Cloud), nunca no servidor dedicado: para ler o de alguém, essa pessoa
// precisa enviar o arquivo.
//
// O layout foi obtido por engenharia reversa das versões 39 e 43 — nenhum
// parser público passa da 37:
//
//   - A partir da versão 38 os quatro contadores (mortes, abates, itens
//     fabricados, construções) viraram um array de estatísticas em float.
//   - O minimapa é gzip, dentro do bloco de cada mundo.
//   - Depois do playerID vêm modificadores de mundo e chaves globais, cujo
//     layout não está decodificado; o inventário que vem depois é localizado
//     por assinatura.
type Character struct {
	Version  int32
	Stats    []float32
	Worlds   map[int64]*WorldPlayerData
	Name     string
	PlayerID int64

	// Inventory é obtido por tentativa: o bloco é procurado por assinatura,
	// porque a estrutura que o precede ainda não foi mapeada. Nulo quando
	// nada decodificou de forma consistente.
	Inventory *Inventory
}

// WorldPlayerData é o que um personagem lembra de um mundo.
type WorldPlayerData struct {
	HaveCustomSpawnPoint bool
	SpawnPoint           Vec3
	HasLogoutPoint       bool
	LogoutPoint          Vec3
	HasDeathPoint        bool
	DeathPoint           Vec3
	HomePoint            Vec3
	Map                  *MinimapData
}

// plausibleWorldCount limita quantos mundos um personagem pode lembrar. Save
// real guarda uns poucos; número maior significa que a leitura saiu do lugar.
func plausibleWorldCount(n int32) bool { return n >= 0 && n <= 64 }

// MinimapData é a névoa de guerra: quais pixels do mapa este jogador
// descobriu. A textura cobre o mundo inteiro, então pixel → mundo é linear.
type MinimapData struct {
	Version          int32
	TextureSize      int32
	Explored         []bool // tamanho TextureSize², true onde foi descoberto
	ExploredByOthers []bool
	ExploredCount    int
}

// statsArrayMinVersion é a primeira versão de personagem observada gravando as
// estatísticas como array de float em vez de quatro contadores. As versões 39
// e 43 foram conferidas contra saves reais; a 38 é suposição não testada.
const statsArrayMinVersion = 38

// ParseCharacterFile lê e decodifica um save de personagem `.fch`.
func ParseCharacterFile(path string) (*Character, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseCharacter(data)
}

// ParseCharacter decodifica um save de personagem `.fch` já em memória.
func ParseCharacter(data []byte) (*Character, error) {
	r := NewReader(data)

	size, err := r.Int32()
	if err != nil {
		return nil, fmt.Errorf("lendo tamanho: %w", err)
	}
	if int(size)+4 > len(data) {
		return nil, fmt.Errorf("arquivo truncado: cabecalho diz %d bytes, arquivo tem %d", size, len(data))
	}

	c := &Character{Worlds: map[int64]*WorldPlayerData{}}
	if c.Version, err = r.Int32(); err != nil {
		return nil, fmt.Errorf("lendo versao: %w", err)
	}

	if c.Version >= statsArrayMinVersion {
		n, err := r.Int32()
		if err != nil {
			return nil, fmt.Errorf("lendo quantidade de estatisticas: %w", err)
		}
		if n < 0 || n > 4096 {
			return nil, fmt.Errorf("quantidade de estatisticas implausivel (%d) — layout da versao %d provavelmente mudou", n, c.Version)
		}
		c.Stats = make([]float32, n)
		for i := range c.Stats {
			if c.Stats[i], err = r.Float32(); err != nil {
				return nil, fmt.Errorf("estatistica %d: %w", i, err)
			}
		}
	} else {
		for range 4 {
			if _, err := r.Int32(); err != nil {
				return nil, err
			}
		}
	}

	// A versão 43 tem um byte a mais entre o array de estatísticas e a
	// contagem de mundos que a 39 não tem. Só foi possível amostrar 39 e 43,
	// então a versão que introduziu o byte é desconhecida: em vez de chutar um
	// corte, tenta o layout antigo e refaz com o novo quando a contagem sai
	// sem sentido — errar aqui nunca é sutil.
	before := r.Pos()
	numWorlds, err := r.Int32()
	if err != nil {
		return nil, fmt.Errorf("lendo quantidade de mundos: %w", err)
	}
	if !plausibleWorldCount(numWorlds) {
		if err := r.Seek(before + 1); err != nil {
			return nil, err
		}
		if numWorlds, err = r.Int32(); err != nil {
			return nil, fmt.Errorf("lendo quantidade de mundos (layout novo): %w", err)
		}
	}
	if !plausibleWorldCount(numWorlds) {
		return nil, fmt.Errorf("quantidade de mundos implausivel (%d) perto do offset %d — layout da versao %d divergiu do conhecido", numWorlds, before, c.Version)
	}

	for i := int32(0); i < numWorlds; i++ {
		key, err := r.Int64()
		if err != nil {
			return nil, fmt.Errorf("mundo %d: %w", i, err)
		}
		wpd, err := readWorldPlayerData(r, c.Version)
		if err != nil {
			return nil, fmt.Errorf("mundo %d (offset %d): %w", i, r.Pos(), err)
		}
		c.Worlds[key] = wpd
	}

	if c.Name, err = r.Str(); err != nil {
		return nil, fmt.Errorf("lendo nome: %w", err)
	}
	if c.PlayerID, err = r.Int64(); err != nil {
		return nil, fmt.Errorf("lendo playerID: %w", err)
	}

	// Daqui para frente (modificadores de mundo, chaves globais, PlayerData)
	// nada está mapeado, então o inventário é recuperado por assinatura.
	c.Inventory = findInventory(data[r.Pos():])

	return c, nil
}

func readWorldPlayerData(r *Reader, version int32) (*WorldPlayerData, error) {
	w := &WorldPlayerData{}
	var err error

	if w.HaveCustomSpawnPoint, err = r.Bool(); err != nil {
		return nil, err
	}
	if w.SpawnPoint, err = r.Vec3(); err != nil {
		return nil, err
	}
	if w.HasLogoutPoint, err = r.Bool(); err != nil {
		return nil, err
	}
	if w.LogoutPoint, err = r.Vec3(); err != nil {
		return nil, err
	}
	if version >= 30 {
		if w.HasDeathPoint, err = r.Bool(); err != nil {
			return nil, err
		}
		if w.DeathPoint, err = r.Vec3(); err != nil {
			return nil, err
		}
	}
	if w.HomePoint, err = r.Vec3(); err != nil {
		return nil, err
	}

	if version < 29 {
		return w, nil
	}
	hasMap, err := r.Bool()
	if err != nil {
		return nil, err
	}
	if !hasMap {
		return w, nil
	}

	blobLen, err := r.Int32()
	if err != nil {
		return nil, fmt.Errorf("tamanho do bloco de mapa: %w", err)
	}
	blob, err := r.Bytes(int(blobLen))
	if err != nil {
		return nil, fmt.Errorf("bloco de mapa (%d bytes): %w", blobLen, err)
	}
	if w.Map, err = parseMinimap(blob); err != nil {
		return nil, fmt.Errorf("minimapa: %w", err)
	}
	return w, nil
}

func parseMinimap(blob []byte) (*MinimapData, error) {
	r := NewReader(blob)
	m := &MinimapData{}

	var err error
	if m.Version, err = r.Int32(); err != nil {
		return nil, err
	}

	compressedLen, err := r.Int32()
	if err != nil {
		return nil, err
	}
	gz, err := r.Bytes(int(compressedLen))
	if err != nil {
		return nil, fmt.Errorf("blob comprimido (%d bytes): %w", compressedLen, err)
	}

	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, fmt.Errorf("abrindo gzip: %w", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("descomprimindo: %w", err)
	}

	rr := NewReader(raw)
	if m.TextureSize, err = rr.Int32(); err != nil {
		return nil, err
	}
	if m.TextureSize <= 0 || m.TextureSize > 8192 {
		return nil, fmt.Errorf("textureSize implausivel: %d", m.TextureSize)
	}

	n := int(m.TextureSize) * int(m.TextureSize)
	m.Explored = make([]bool, n)
	for i := range m.Explored {
		b, err := rr.Byte()
		if err != nil {
			return nil, fmt.Errorf("pixel %d de %d: %w", i, n, err)
		}
		if b != 0 {
			m.Explored[i] = true
			m.ExploredCount++
		}
	}

	// Save recente traz um segundo plano: o que outros jogadores descobriram e
	// compartilharam. Não existe nos antigos, então acabar o dado aqui não é
	// erro.
	if rr.Remaining() >= n {
		m.ExploredByOthers = make([]bool, n)
		for i := range m.ExploredByOthers {
			b, _ := rr.Byte()
			m.ExploredByOthers[i] = b != 0
		}
	}
	return m, nil
}

// inventoryVersion é a versão de serialização de itens dos saves atuais.
const inventoryVersion = 106

// findInventory procura um bloco de inventário e devolve o primeiro que
// decodifica limpo e com conteúdo plausível.
//
// É heurística: a estrutura do PlayerData que o precede não está decodificada,
// então não há offset exato para pular.
func findInventory(tail []byte) *Inventory {
	marker := []byte{inventoryVersion, 0, 0, 0}
	for off := 0; off+8 < len(tail); {
		i := bytes.Index(tail[off:], marker)
		if i < 0 {
			return nil
		}
		at := off + i
		r := NewReader(tail[at:])
		if inv, err := parseInventoryFrom(r); err == nil && plausible(inv) {
			return inv
		}
		off = at + 1
	}
	return nil
}

// plausible descarta sequências que só por acaso começam com a assinatura:
// inventário de verdade tem itens e nomes de prefab legíveis.
func plausible(inv *Inventory) bool {
	if len(inv.Items) == 0 || len(inv.Items) > 512 {
		return false
	}
	for _, it := range inv.Items {
		if it.Name == "" || len(it.Name) > 64 || it.Stack < 0 {
			return false
		}
		for _, ch := range it.Name {
			if ch < 'A' || ch > 'z' {
				return false
			}
		}
	}
	return true
}
