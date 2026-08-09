package integracao

// SalvarIntegracaoRequestDto é o corpo de `PUT /configuracao/discord`.
//
// Os campos de segredo (`webhook_url`, `bot_token`) são PONTEIROS: o painel
// nunca recebe o valor atual de volta, então ele também não pode reenviá-lo.
// Ausente (`null`) = "mantenha o que já está gravado"; string vazia = "apague".
// Com string simples, toda edição do nome de exibição apagaria o token.
type SalvarIntegracaoRequestDto struct {
	Habilitado bool    `json:"habilitado"`
	Modo       string  `json:"modo" binding:"required"`
	WebhookURL *string `json:"webhook_url"`
	BotToken   *string `json:"bot_token"`
	CanalID    *string `json:"canal_id"`
	Username   string  `json:"username"`
	// Eventos são os tipos que geram mensagem. Nulo/vazio = a lista padrão.
	Eventos []string `json:"eventos"`
}

// aplicarEm devolve o valor novo de um segredo: o enviado, ou o atual quando o
// campo não veio.
func aplicarEm(atual string, novo *string) string {
	if novo == nil {
		return atual
	}
	return *novo
}
