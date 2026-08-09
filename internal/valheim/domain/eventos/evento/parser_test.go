package evento

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnalisarReconheceAsLinhasDoServidor cobre as linhas que o filtro do
// container deixa passar (`VALHEIM_LOG_FILTER_REGEXP_*`). É o teste que precisa
// ser atualizado quando uma versão do jogo mudar o formato do log — de
// propósito: é aqui que a quebra tem de aparecer.
func TestAnalisarReconheceAsLinhasDoServidor(t *testing.T) {
	casos := []struct {
		nome    string
		linha   string
		tipo    Tipo
		jogador string
		steamID string
		zdoid   string
		texto   string
	}{
		{
			nome:    "personagem entrando",
			linha:   `03/24/2024 22:31:07: Got character ZDOID from Slartibartfast : -1146266487:1`,
			tipo:    TipoEntrou,
			jogador: "Slartibartfast",
			zdoid:   "-1146266487:1",
		},
		{
			nome:    "nome com espaço",
			linha:   `03/24/2024 22:31:07: Got character ZDOID from Erik o Vermelho : 42:1`,
			tipo:    TipoEntrou,
			jogador: "Erik o Vermelho",
			zdoid:   "42:1",
		},
		{
			nome:    "morte é o ZDOID zerado",
			linha:   `03/24/2024 22:40:11: Got character ZDOID from Slartibartfast : 0:0`,
			tipo:    TipoMorreu,
			jogador: "Slartibartfast",
			zdoid:   "", // zerado não identifica objeto nenhum
		},
		{
			nome:    "conexão traz o SteamID e não traz o nome",
			linha:   `03/24/2024 22:30:56: Got connection SteamID 76561198005507113`,
			tipo:    TipoConexao,
			steamID: "76561198005507113",
		},
		{
			nome:  "saída sem hífen em non persistent",
			linha: `03/24/2024 22:50:11: Destroying abandoned non persistent zdo -1146266487:1 own 0`,
			tipo:  TipoSaiu,
			zdoid: "-1146266487:1",
		},
		{
			nome:  "saída com hífen em non-persistent",
			linha: `03/24/2024 22:50:11: Destroying abandoned non-persistent zdo -1146266487:1 own 0`,
			tipo:  TipoSaiu,
			zdoid: "-1146266487:1",
		},
		{
			nome:  "servidor pronto",
			linha: `03/24/2024 22:29:00: Game server connected`,
			tipo:  TipoServidorPronto,
		},
		{
			nome:  "linha fora dos padrões conhecidos é guardada, não descartada",
			linha: `03/24/2024 22:29:00: Something entirely new happened`,
			tipo:  TipoDesconhecido,
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			analise, err := Analisar(caso.linha)

			require.NoError(t, err)
			assert.Equal(t, caso.tipo, analise.Tipo)
			assert.Equal(t, caso.jogador, analise.Jogador)
			assert.Equal(t, caso.steamID, analise.SteamID)
			assert.Equal(t, caso.zdoid, analise.ZDOID)
			assert.Equal(t, caso.texto, analise.Texto)
		})
	}
}

// TestAnalisarExtraiAsMensagensDeChat trata as linhas `Got text`, cujo formato
// varia entre versões do jogo. O contrato é: reconhecer como mensagem SEMPRE, e
// nomear o autor só quando a linha o nomeia.
func TestAnalisarExtraiAsMensagensDeChat(t *testing.T) {
	casos := []struct {
		nome    string
		linha   string
		jogador string
		texto   string
	}{
		{
			nome:    "autor depois do texto",
			linha:   `03/24/2024 23:00:00: Got text "vem pro barco" from Odin`,
			jogador: "Odin",
			texto:   "vem pro barco",
		},
		{
			nome:    "autor antes do texto",
			linha:   `03/24/2024 23:00:00: Got text from Odin: vem pro barco`,
			jogador: "Odin",
			texto:   "vem pro barco",
		},
		{
			nome:  "sem autor identificável",
			linha: `03/24/2024 23:00:00: Got text: vem pro barco`,
			texto: "vem pro barco",
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			analise, err := Analisar(caso.linha)

			require.NoError(t, err)
			assert.Equal(t, TipoMensagem, analise.Tipo)
			assert.Equal(t, caso.jogador, analise.Jogador)
			assert.Equal(t, caso.texto, analise.Texto)
		})
	}
}

// TestAnalisarLeOCarimboDeHora confere a conversão do carimbo do servidor —
// que vem em MM/DD/AAAA, sem fuso, na hora local da máquina.
func TestAnalisarLeOCarimboDeHora(t *testing.T) {
	t.Run("carimbo MM/DD/AAAA vira UTC", func(t *testing.T) {
		analise, err := Analisar(`03/24/2024 22:31:07: Game server connected`)

		require.NoError(t, err)
		require.True(t, analise.TemHora)
		esperado := time.Date(2024, 3, 24, 22, 31, 7, 0, time.Local).UTC()
		assert.Equal(t, esperado, analise.OcorridoEm)
	})

	t.Run("dia acima de 12 cai no layout DD/MM/AAAA", func(t *testing.T) {
		analise, err := Analisar(`24/03/2024 22:31:07: Game server connected`)

		require.NoError(t, err)
		require.True(t, analise.TemHora)
		esperado := time.Date(2024, 3, 24, 22, 31, 7, 0, time.Local).UTC()
		assert.Equal(t, esperado, analise.OcorridoEm)
	})

	t.Run("linha sem carimbo não inventa hora", func(t *testing.T) {
		analise, err := Analisar(`Game server connected`)

		require.NoError(t, err)
		assert.False(t, analise.TemHora)
		assert.True(t, analise.OcorridoEm.IsZero())
		assert.Equal(t, TipoServidorPronto, analise.Tipo)
	})
}

// TestAnalisarLinhasReaisDoServidor usa linhas copiadas do log de um servidor
// em produção (Valheim em `ghcr.io/community-valheim-tools/valheim-server`,
// agosto de 2026), COM o prefixo do supervisor que o container encaminha.
//
// É o teste que pega a regressão que os casos sintéticos não pegam: o carimbo
// de hora não está no começo da linha.
func TestAnalisarLinhasReaisDoServidor(t *testing.T) {
	const prefixo = "Aug  8 18:54:11 supervisord: valheim-server "

	t.Run("entrada com prefixo do supervisor", func(t *testing.T) {
		analise, err := Analisar(prefixo + "08/08/2026 18:54:11: Got character ZDOID from Espen Lindberg : 1353269051:18575")

		require.NoError(t, err)
		assert.Equal(t, TipoEntrou, analise.Tipo)
		assert.Equal(t, "Espen Lindberg", analise.Jogador)
		assert.Equal(t, "1353269051:18575", analise.ZDOID)
		assert.Equal(t, "1353269051", analise.Dono)
		assert.True(t, analise.TemHora, "a hora do JOGO precisa sobreviver ao prefixo")
		assert.Equal(t,
			time.Date(2026, 8, 8, 18, 54, 11, 0, time.Local).UTC(),
			analise.OcorridoEm)
	})

	t.Run("nome com acento", func(t *testing.T) {
		analise, err := Analisar(prefixo + "08/08/2026 19:19:54: Got character ZDOID from Gunnar Úlfhéðin : 1814428521:1")

		require.NoError(t, err)
		assert.Equal(t, TipoEntrou, analise.Tipo)
		assert.Equal(t, "Gunnar Úlfhéðin", analise.Jogador)
	})

	t.Run("morte com prefixo", func(t *testing.T) {
		analise, err := Analisar(prefixo + "08/08/2026 18:54:03: Got character ZDOID from Espen Lindberg : 0:0")

		require.NoError(t, err)
		assert.Equal(t, TipoMorreu, analise.Tipo)
		assert.Equal(t, "Espen Lindberg", analise.Jogador)
	})

	t.Run("objeto abandonado traz o dono", func(t *testing.T) {
		analise, err := Analisar(prefixo + "08/08/2026 19:23:29: Destroying abandoned non persistent zdo -53465420:2236 owner -53465420")

		require.NoError(t, err)
		assert.Equal(t, TipoSaiu, analise.Tipo)
		assert.Equal(t, "-53465420:2236", analise.ZDOID)
		assert.Equal(t, "-53465420", analise.Dono,
			"é o dono que amarra as dezenas de linhas de uma saída a uma pessoa só")
	})

	t.Run("conexão", func(t *testing.T) {
		analise, err := Analisar(prefixo + "08/08/2026 19:03:56: Got connection SteamID 76561198821824340")

		require.NoError(t, err)
		assert.Equal(t, TipoConexao, analise.Tipo)
		assert.Equal(t, "76561198821824340", analise.SteamID)
	})

	t.Run("servidor no ar", func(t *testing.T) {
		analise, err := Analisar(prefixo + "08/09/2026 05:10:26: Game server connected")

		require.NoError(t, err)
		assert.Equal(t, TipoServidorPronto, analise.Tipo)
	})
}

// TestAnalisarRecusaLinhaVazia — é o único erro que o parser conhece.
func TestAnalisarRecusaLinhaVazia(t *testing.T) {
	for _, linha := range []string{"", "   ", "\n\t "} {
		_, err := Analisar(linha)
		assert.ErrorIs(t, err, ErrLinhaVazia)
	}
}
