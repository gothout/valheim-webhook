package ingestao

import (
	"strings"

	"valheim-webhook/internal/infra/notificador/discord"
)

// Tipos de evento tratados aqui.
//
// São strings, e não o tipo do subdomínio de eventos, pelo mesmo motivo do
// `Registro.Tipo`: o vocabulário é de lá, e o valor é o que atravessa. As
// constantes existem para o compilador pegar erro de digitação neste arquivo.
const (
	tipoEntrou         = "entrou"
	tipoSaiu           = "saiu"
	tipoMorreu         = "morreu"
	tipoMensagem       = "mensagem"
	tipoConexao        = "conexao"
	tipoServidorPronto = "servidor_pronto"
)

// Cores dos cartões do Discord.
//
// Elas contam a história antes de a pessoa ler: verde chegou, cinza foi embora,
// vermelho morreu, azul falou. Quem olha o canal de relance entende sem ler.
const (
	corEntrou   = 0x43B581
	corSaiu     = 0x99AAB5
	corMorreu   = 0xE74C3C
	corMensagem = 0x5865F2
	corServidor = 0xF1C40F
	corPadrao   = 0x2C2F33
)

// paraMensagem monta o cartão do Discord para um evento.
//
// Devolve `false` quando o evento não rende mensagem — hoje, só o caso da saída
// de um personagem que ninguém conseguiu nomear: "alguém saiu" não é notícia.
func paraMensagem(r Registro, p Presenca) (discord.Mensagem, bool) {
	jogador := primeiroNaoVazio(r.Jogador, p.Nome)

	embed := &discord.Embed{
		Cor:    corPadrao,
		Quando: r.OcorridoEm,
		Rodape: r.Servidor,
	}

	switch r.Tipo {
	case tipoEntrou:
		embed.Titulo = "⚔️ " + jogador + " entrou no mundo"
		embed.Cor = corEntrou

	case tipoSaiu:
		if jogador == "" {
			return discord.Mensagem{}, false
		}
		embed.Titulo = "🚪 " + jogador + " saiu"
		embed.Cor = corSaiu

	case tipoMorreu:
		embed.Titulo = "💀 " + jogador + " morreu"
		embed.Descricao = "Valhalla pode esperar."
		embed.Cor = corMorreu

	case tipoMensagem:
		embed.Cor = corMensagem
		if jogador != "" {
			embed.Titulo = "💬 " + jogador
		} else {
			embed.Titulo = "💬 mensagem no mundo"
		}
		// O texto do jogador entra como DESCRIÇÃO, nunca como conteúdo solto da
		// mensagem: dentro do embed ele não vira menção, não vira link
		// pré-visualizado e não vira formatação — e as menções já estão
		// desligadas no publicador (`allowed_mentions`).
		embed.Descricao = citar(r.Texto)

	case tipoServidorPronto:
		embed.Titulo = "🌍 Servidor no ar"
		embed.Descricao = "O mundo subiu e está aceitando jogadores."
		embed.Cor = corServidor

	case tipoConexao:
		embed.Titulo = "🔌 Nova conexão"
		embed.Cor = corPadrao

	default:
		embed.Titulo = "📄 " + r.Rotulo
		embed.Descricao = citar(r.Linha)
	}

	return discord.Mensagem{Embed: embed}, true
}

// citar formata o texto do jogador como citação, escapando o que o Discord
// interpretaria como formatação.
func citar(texto string) string {
	limpo := strings.TrimSpace(texto)
	if limpo == "" {
		return ""
	}
	// Uma linha só: quebra de linha dentro de uma citação do Discord sai da
	// citação e o resto vira texto normal — um jogador poderia forjar o
	// formato de uma mensagem do sistema.
	limpo = strings.ReplaceAll(limpo, "\n", " ")
	limpo = strings.ReplaceAll(limpo, "\r", " ")
	return "> " + escapar(limpo)
}

// escapar neutraliza a marcação do Discord no texto vindo do jogo.
func escapar(texto string) string {
	trocador := strings.NewReplacer(
		`\`, `\\`,
		"`", "\\`",
		`*`, `\*`,
		`_`, `\_`,
		`~`, `\~`,
		`|`, `\|`,
		`>`, `\>`,
	)
	return trocador.Replace(texto)
}
