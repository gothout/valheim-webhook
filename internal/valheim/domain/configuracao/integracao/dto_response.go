package integracao

import (
	"strings"
	"time"
)

// IntegracaoResponseDto é a configuração como o painel a recebe.
//
// Nenhum segredo sai daqui em texto claro. O que o painel precisa saber é
// (a) se está preenchido e (b) o suficiente para a pessoa reconhecer QUAL
// valor está lá — e para isso a máscara basta.
type IntegracaoResponseDto struct {
	Servico    string `json:"servico"`
	Habilitado bool   `json:"habilitado"`
	Modo       string `json:"modo"`
	Username   string `json:"username"`

	WebhookConfigurado bool   `json:"webhook_configurado"`
	WebhookMascara     string `json:"webhook_mascara,omitempty"`
	TokenConfigurado   bool   `json:"token_configurado"`
	TokenMascara       string `json:"token_mascara,omitempty"`
	CanalID            string `json:"canal_id,omitempty"`

	Eventos []string `json:"eventos"`
	// Ativa é o resultado das duas perguntas juntas — é o que acende o sinal
	// verde na tela.
	Ativa        bool      `json:"ativa"`
	AtualizadoEm time.Time `json:"atualizado_em"`
}

// ParaResponse converte a entidade, mascarando os segredos.
func ParaResponse(i Integracao, eventosPadrao []string) IntegracaoResponseDto {
	eventos := i.ListaDeEventos()
	if len(eventos) == 0 {
		eventos = eventosPadrao
	}

	return IntegracaoResponseDto{
		Servico:            i.Servico,
		Habilitado:         i.Habilitado,
		Modo:               i.Modo,
		Username:           i.Username,
		WebhookConfigurado: strings.TrimSpace(i.WebhookURL) != "",
		WebhookMascara:     mascarar(i.WebhookURL),
		TokenConfigurado:   strings.TrimSpace(i.BotToken) != "",
		TokenMascara:       mascarar(i.BotToken),
		CanalID:            i.CanalID,
		Eventos:            eventos,
		Ativa:              i.Ativa(),
		AtualizadoEm:       i.AtualizadoEm,
	}
}

// mascarar mostra só o fim do segredo.
//
// O FIM, e não o começo: o começo de um webhook do Discord é igual em todos
// eles (`https://discord.com/api/webhooks/…`) e não distingue nada. Segredo
// curto demais para mascarar com segurança vira só pontinhos.
func mascarar(segredo string) string {
	limpo := strings.TrimSpace(segredo)
	if limpo == "" {
		return ""
	}
	if len(limpo) <= 8 {
		return "••••"
	}
	return "••••" + limpo[len(limpo)-4:]
}
