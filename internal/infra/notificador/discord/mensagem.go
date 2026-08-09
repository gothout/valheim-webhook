package discord

import "time"

// Mensagem é o que o resto do sistema pede para publicar no Discord.
//
// É o vocabulário DESTE pacote, não o do Discord: quem chama monta título,
// descrição e campos, e a tradução para o JSON da API acontece aqui dentro. É o
// que permite trocar webhook por bot (ou por outro chat) sem tocar em nenhum
// subdomínio.
type Mensagem struct {
	// Conteudo é o texto solto da mensagem. Vazio quando só há embed.
	Conteudo string
	// Embed é o cartão colorido. Opcional.
	Embed *Embed
}

// Embed é o cartão da mensagem.
type Embed struct {
	Titulo    string
	Descricao string
	// Cor é o inteiro RGB (0xRRGGBB) da barra lateral do cartão.
	Cor    int
	Campos []Campo
	Rodape string
	// Quando é o carimbo exibido no rodapé. Zero omite.
	Quando time.Time
}

// Campo é um par nome/valor do cartão.
type Campo struct {
	Nome    string
	Valor   string
	EmLinha bool
}

// Limites da API do Discord. Estourar qualquer um deles faz a API devolver 400
// e a mensagem some — por isso o corte acontece aqui, antes de sair.
const (
	MaxConteudo  = 2000
	MaxTitulo    = 256
	MaxDescricao = 4096
	MaxCampos    = 25
	MaxNomeCampo = 256
	MaxValor     = 1024
	MaxRodape    = 2048
)

// corpo é o JSON aceito pelas duas rotas (webhook e canal do bot).
type corpo struct {
	Username        string           `json:"username,omitempty"`
	Content         string           `json:"content,omitempty"`
	Embeds          []corpoEmbed     `json:"embeds,omitempty"`
	AllowedMentions *allowedMentions `json:"allowed_mentions"`
}

// allowedMentions com `parse` vazio DESLIGA toda menção da mensagem.
//
// Isto não é detalhe de estilo, é segurança: o conteúdo que passa por aqui vem
// do chat de dentro do jogo e do nome que o jogador escolheu. Sem esta trava,
// qualquer um digitaria `@everyone` no Valheim e tocaria o sino de todo mundo
// no servidor do Discord.
type allowedMentions struct {
	Parse []string `json:"parse"`
}

type corpoEmbed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []corpoCampo `json:"fields,omitempty"`
	Footer      *corpoRodape `json:"footer,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
}

type corpoCampo struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type corpoRodape struct {
	Text string `json:"text"`
}

// paraCorpo traduz a Mensagem para o JSON da API, já dentro dos limites.
func paraCorpo(m Mensagem, username string) corpo {
	c := corpo{
		Username:        username,
		Content:         cortar(m.Conteudo, MaxConteudo),
		AllowedMentions: &allowedMentions{Parse: []string{}},
	}

	if m.Embed == nil {
		return c
	}

	e := corpoEmbed{
		Title:       cortar(m.Embed.Titulo, MaxTitulo),
		Description: cortar(m.Embed.Descricao, MaxDescricao),
		Color:       m.Embed.Cor,
	}
	if m.Embed.Rodape != "" {
		e.Footer = &corpoRodape{Text: cortar(m.Embed.Rodape, MaxRodape)}
	}
	if !m.Embed.Quando.IsZero() {
		e.Timestamp = m.Embed.Quando.UTC().Format(time.RFC3339)
	}

	campos := m.Embed.Campos
	if len(campos) > MaxCampos {
		campos = campos[:MaxCampos]
	}
	for _, campo := range campos {
		valor := cortar(campo.Valor, MaxValor)
		if valor == "" {
			// A API recusa campo com valor vazio e derruba a mensagem inteira.
			valor = "—"
		}
		e.Fields = append(e.Fields, corpoCampo{
			Name:   cortar(campo.Nome, MaxNomeCampo),
			Value:  valor,
			Inline: campo.EmLinha,
		})
	}

	c.Embeds = []corpoEmbed{e}
	return c
}

// cortar limita o texto sem partir rune ao meio.
func cortar(s string, limite int) string {
	if len(s) <= limite {
		return s
	}
	runas := []rune(s)
	if len(runas) <= limite {
		return s
	}
	return string(runas[:limite-1]) + "…"
}
