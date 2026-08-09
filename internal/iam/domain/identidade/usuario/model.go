// Package usuario guarda quem pode entrar no painel.
//
// Entidade: Usuario. Dependências: Postgres (repository), bcrypt (hash).
//
// Regra que organiza o pacote: **a senha não sai daqui**. O hash tem
// `json:"-"`, não entra em DTO nenhum e a comparação é um método do service —
// quem quer saber se uma senha confere pergunta, não recebe o hash para
// comparar por conta própria.
package usuario

import (
	"time"

	"github.com/google/uuid"

	"valheim-webhook/internal/pkg/papel"
)

// Identidade do subdomínio (observador de erros e logs de boot).
const (
	Sistema    = "iam"
	Dominio    = "identidade"
	Subdominio = "usuario"
)

// Usuario é quem acessa o painel.
type Usuario struct {
	UUID  uuid.UUID `gorm:"column:uuid;type:uuid;primaryKey" json:"uuid"`
	Nome  string    `gorm:"column:nome;not null" json:"nome"`
	Email string    `gorm:"column:email;not null;uniqueIndex" json:"email"`
	// SenhaHash nunca sai do pacote: `json:"-"` o esconde de qualquer resposta
	// e nenhum DTO o carrega.
	SenhaHash string `gorm:"column:senha_hash;not null" json:"-"`
	// Papel é o vocabulário do pacote `papel` (admin | visualizador).
	Papel papel.Papel `gorm:"column:papel;not null" json:"papel"`
	// Ativo desligado é o "demitido sem apagar": o histórico de quem fez o quê
	// continua fazendo sentido, e a pessoa não entra mais.
	Ativo          bool       `gorm:"column:ativo;not null;default:true" json:"ativo"`
	UltimoAcessoEm *time.Time `gorm:"column:ultimo_acesso_em" json:"ultimo_acesso_em,omitempty"`

	CriadoEm     time.Time `gorm:"column:criado_em;autoCreateTime" json:"criado_em"`
	AtualizadoEm time.Time `gorm:"column:atualizado_em;autoUpdateTime" json:"atualizado_em"`
}

// TableName fixa o nome da tabela no padrão `{sistema}_{dominio}_{subdominio}`.
func (Usuario) TableName() string { return "iam_identidade_usuario" }

// PodeEntrar diz se a conta está em condições de abrir sessão.
func (u Usuario) PodeEntrar() bool { return u.Ativo }

// MinimoDaSenha é o tamanho mínimo aceito.
//
// Oito caracteres, sem exigência de símbolo ou maiúscula: a regra que se sabe
// que funciona é comprimento, e as outras só produzem `Senha123!` em todo
// lugar. Quem instala isto é o dono de um servidor de Valheim, não um banco.
const MinimoDaSenha = 8

// MaximoDaSenha existe por causa do bcrypt, que ignora tudo além de 72 BYTES —
// aceitar 200 caracteres e usar 72 seria mentir para quem escolheu a senha.
const MaximoDaSenha = 72

// papelAdmin é o atalho interno para o papel administrativo (usado nas
// consultas e nas regras do último administrador).
const papelAdmin = papel.Admin
