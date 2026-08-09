package auth

import (
	"time"

	"github.com/google/uuid"

	"valheim-webhook/internal/pkg/papel"
)

// ContaResponseDto é quem está logado, como o painel o recebe.
type ContaResponseDto struct {
	UUID        uuid.UUID   `json:"uuid"`
	Nome        string      `json:"nome"`
	Email       string      `json:"email"`
	Papel       papel.Papel `json:"papel"`
	PapelRotulo string      `json:"papel_rotulo"`
	// Admin é redundante com `papel`, e existe para o painel não precisar
	// conhecer o vocabulário de papéis só para decidir se mostra o menu de
	// administração.
	Admin          bool       `json:"admin"`
	UltimoAcessoEm *time.Time `json:"ultimo_acesso_em,omitempty"`
}

// SessaoResponseDto é a resposta do login.
//
// O TOKEN não vai no corpo: ele viaja no cookie `HttpOnly`, e devolvê-lo
// também aqui daria a qualquer script da página a chance de guardá-lo em
// `localStorage` — que é exatamente o que o `HttpOnly` existe para impedir.
type SessaoResponseDto struct {
	Usuario  ContaResponseDto `json:"usuario"`
	ExpiraEm time.Time        `json:"expira_em"`
}

// ParaContaResponse converte a conta.
func ParaContaResponse(c Conta) ContaResponseDto {
	return ContaResponseDto{
		UUID:           c.UUID,
		Nome:           c.Nome,
		Email:          c.Email,
		Papel:          c.Papel,
		PapelRotulo:    c.Papel.Rotulo(),
		Admin:          c.Papel.EhAdmin(),
		UltimoAcessoEm: c.UltimoAcessoEm,
	}
}
