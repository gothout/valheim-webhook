package valheimsave

import (
	"os"
	"testing"
)

// Os valores esperados vieram de um save real decodificado pela implementação
// de referência em Java (Kakoen/valheim-save-tools): divergir aqui significa
// que o hash saiu do que o jogo faz.
func TestStableHash(t *testing.T) {
	cases := []struct {
		name string
		want int32
	}{
		{"piece_chest_wood", 328745978},
	}
	for _, c := range cases {
		if got := StableHash(c.name); got != c.want {
			t.Errorf("StableHash(%q) = %d, esperado %d", c.name, got, c.want)
		}
	}
}

func TestLookupName(t *testing.T) {
	if got := LookupName(328745978); got != "piece_chest_wood" {
		t.Errorf("LookupName(328745978) = %q, esperado \"piece_chest_wood\"", got)
	}
	if got := LookupName(1); got != "" {
		t.Errorf("hash desconhecido deveria devolver string vazia, veio %q", got)
	}
}

// Save de mundo e de personagem não são versionados: o `.db` tem dezenas de
// megabytes e o `.fch` é arquivo pessoal de jogador (nome, playerID,
// inventário, mapa). Estes testes rodam quando alguém põe os arquivos em
// `testdata/` e pulam sozinhos quando não estão lá.
func TestParseWorld(t *testing.T) {
	const path = "testdata/playground.db"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sem %s por perto, pulando", path)
	}

	w, err := ParseWorldFile(path)
	if err != nil {
		t.Fatalf("ParseWorldFile: %v", err)
	}
	if w.Version < MinWorldVersion {
		t.Errorf("versao = %d, abaixo do minimo %d", w.Version, MinWorldVersion)
	}
	if len(w.ZDOs) == 0 {
		t.Fatal("nenhum zdo decodificado")
	}

	containers, bad := 0, 0
	for i := range w.ZDOs {
		_, ok, err := w.ZDOs[i].Inventory()
		if !ok {
			continue
		}
		containers++
		if err != nil {
			bad++
			t.Logf("container %d (%s): %v", i, w.ZDOs[i].Name(), err)
		}
	}
	if bad != 0 {
		t.Errorf("%d de %d inventarios falharam ao decodificar", bad, containers)
	}
	t.Logf("%d zdos, %d containers", len(w.ZDOs), containers)
}

// Cobre os dois layouts de personagem encontrados em saves reais: a 39, que
// emenda a contagem de mundos direto no array de estatísticas, e a 43, que põe
// um byte entre os dois.
func TestParseCharacter(t *testing.T) {
	matches, err := os.ReadDir("testdata")
	if err != nil {
		t.Skip("sem testdata/, pulando")
	}

	found := 0
	for _, e := range matches {
		if e.IsDir() || len(e.Name()) < 4 || e.Name()[len(e.Name())-4:] != ".fch" {
			continue
		}
		found++
		t.Run(e.Name(), func(t *testing.T) {
			c, err := ParseCharacterFile("testdata/" + e.Name())
			if err != nil {
				t.Fatalf("ParseCharacterFile: %v", err)
			}
			if c.Name == "" {
				t.Error("nome do personagem vazio")
			}
			if c.PlayerID == 0 {
				t.Error("playerID zerado")
			}
			if len(c.Worlds) == 0 {
				t.Error("nenhum mundo decodificado")
			}
			for key, w := range c.Worlds {
				if w.Map == nil {
					continue
				}
				if w.Map.TextureSize != 2048 {
					t.Errorf("mundo %d: textureSize = %d, esperado 2048", key, w.Map.TextureSize)
				}
				if len(w.Map.Explored) != int(w.Map.TextureSize)*int(w.Map.TextureSize) {
					t.Errorf("mundo %d: plano de exploracao com tamanho errado", key)
				}
			}
		})
	}
	if found == 0 {
		t.Skip("nenhum .fch em testdata/, pulando")
	}
}
