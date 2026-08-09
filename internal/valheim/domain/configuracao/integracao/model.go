// Package integracao guarda, no banco, a configuração da integração com o
// Discord — a mesma que o `configs.json` traz, só que editável pelo painel.
//
// Entidade: Integracao. Dependências: Postgres (repository) e o publicador do
// Discord (infra), que é RECONFIGURADO quando alguém salva.
//
// Por que a configuração vive em dois lugares: o arquivo é a SEMENTE (o que
// sobe numa instalação nova, e o que um deploy automatizado sabe entregar) e o
// banco é a VERDADE a partir do primeiro salvamento pelo painel. Sem o arquivo,
// não haveria como subir configurado; sem o banco, mudar o canal do Discord
// exigiria acesso ao servidor.
package integracao

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Identidade do subdomínio (observador de erros e logs de boot).
const (
	Sistema    = "valheim"
	Dominio    = "configuracao"
	Subdominio = "integracao"
)

// ServicoDiscord é a chave da única integração que existe hoje.
//
// A tabela é chaveada por SERVIÇO, e não uma tabela de uma linha só, porque a
// segunda integração (Telegram, um webhook genérico) não deve exigir migration
// nova — só uma linha nova.
const ServicoDiscord = "discord"

// Modos de envio.
const (
	// ModoBot usa a API do app do Discord (token do bot + canal).
	ModoBot = "bot"
	// ModoWebhook usa um webhook de canal.
	ModoWebhook = "webhook"
)

// Integracao é a configuração persistida de um serviço externo.
type Integracao struct {
	UUID    uuid.UUID `gorm:"column:uuid;type:uuid;primaryKey" json:"uuid"`
	Servico string    `gorm:"column:servico;not null;uniqueIndex" json:"servico"`

	Habilitado bool   `gorm:"column:habilitado;not null;default:false" json:"habilitado"`
	Modo       string `gorm:"column:modo;not null;default:'webhook'" json:"modo"`

	// Os três campos abaixo são SEGREDOS: têm `json:"-"` e nunca entram numa
	// resposta. O painel recebe apenas a máscara e o "está preenchido?".
	WebhookURL string `gorm:"column:webhook_url;not null;default:''" json:"-"`
	BotToken   string `gorm:"column:bot_token;not null;default:''" json:"-"`
	CanalID    string `gorm:"column:canal_id;not null;default:''" json:"-"`

	Username string `gorm:"column:username;not null;default:''" json:"username"`
	// Eventos é a lista de tipos que geram mensagem, gravada separada por
	// vírgula. Vazio = a lista padrão do subdomínio de eventos.
	Eventos string `gorm:"column:eventos;not null;default:''" json:"eventos"`

	AtualizadoPor *uuid.UUID `gorm:"column:atualizado_por;type:uuid" json:"atualizado_por,omitempty"`
	CriadoEm      time.Time  `gorm:"column:criado_em;autoCreateTime" json:"criado_em"`
	AtualizadoEm  time.Time  `gorm:"column:atualizado_em;autoUpdateTime" json:"atualizado_em"`
}

// TableName fixa o nome da tabela no padrão `{sistema}_{dominio}_{subdominio}`.
func (Integracao) TableName() string { return "valheim_configuracao_integracao" }

// ListaDeEventos devolve os tipos configurados, já limpos.
func (i Integracao) ListaDeEventos() []string {
	var lista []string
	for _, pedaco := range strings.Split(i.Eventos, ",") {
		if limpo := strings.TrimSpace(pedaco); limpo != "" {
			lista = append(lista, limpo)
		}
	}
	return lista
}

// TemDestino diz se há para onde mandar mensagem no modo escolhido.
func (i Integracao) TemDestino() bool {
	if i.Modo == ModoBot {
		return strings.TrimSpace(i.BotToken) != "" && strings.TrimSpace(i.CanalID) != ""
	}
	return strings.TrimSpace(i.WebhookURL) != ""
}

// Ativa junta as duas perguntas: ligada E com destino.
func (i Integracao) Ativa() bool { return i.Habilitado && i.TemDestino() }
