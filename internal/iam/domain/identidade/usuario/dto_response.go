package usuario

import (
	"time"

	"github.com/google/uuid"

	"valheim-webhook/internal/pkg/papel"
)

// UsuarioResponseDto é o usuário como a API o devolve.
//
// Não existe campo de senha aqui — nem o hash, nem nada derivado dele. É a
// regra do pacote, e ela é garantida por construção: o DTO é montado campo a
// campo, nunca por serialização direta da entidade.
type UsuarioResponseDto struct {
	UUID           uuid.UUID   `json:"uuid"`
	Nome           string      `json:"nome"`
	Email          string      `json:"email"`
	Papel          papel.Papel `json:"papel"`
	PapelRotulo    string      `json:"papel_rotulo"`
	Ativo          bool        `json:"ativo"`
	UltimoAcessoEm *time.Time  `json:"ultimo_acesso_em,omitempty"`
	CriadoEm       time.Time   `json:"criado_em"`
}

// ParaResponse converte a entidade.
func ParaResponse(u Usuario) UsuarioResponseDto {
	return UsuarioResponseDto{
		UUID:           u.UUID,
		Nome:           u.Nome,
		Email:          u.Email,
		Papel:          u.Papel,
		PapelRotulo:    u.Papel.Rotulo(),
		Ativo:          u.Ativo,
		UltimoAcessoEm: u.UltimoAcessoEm,
		CriadoEm:       u.CriadoEm,
	}
}

// ParaResponseLista converte a lista inteira.
func ParaResponseLista(usuarios []Usuario) []UsuarioResponseDto {
	dtos := make([]UsuarioResponseDto, 0, len(usuarios))
	for _, u := range usuarios {
		dtos = append(dtos, ParaResponse(u))
	}
	return dtos
}
