package usuario

import "valheim-webhook/internal/pkg/papel"

// CriarUsuarioRequestDto é o corpo de `POST /usuarios`.
type CriarUsuarioRequestDto struct {
	Nome  string `json:"nome" binding:"required"`
	Email string `json:"email" binding:"required"`
	Senha string `json:"senha" binding:"required"`
	// Papel vazio vira `visualizador`: o padrão de um convite é dar o MENOR
	// acesso, não o maior.
	Papel string `json:"papel"`
}

// PapelEscolhido devolve o papel pedido, com o padrão seguro.
func (d CriarUsuarioRequestDto) PapelEscolhido() (papel.Papel, error) {
	if d.Papel == "" {
		return papel.Visualizador, nil
	}
	p, ok := papel.De(d.Papel)
	if !ok {
		return "", ErrPapelInvalido
	}
	return p, nil
}

// AtualizarUsuarioRequestDto é o corpo de `PATCH /usuarios/{uuid}`.
//
// `Ativo` é ponteiro para distinguir "não mandou o campo" de "mandou false" —
// com bool simples, toda atualização de nome desativaria o usuário sem querer.
type AtualizarUsuarioRequestDto struct {
	Nome  string `json:"nome" binding:"required"`
	Papel string `json:"papel" binding:"required"`
	Ativo *bool  `json:"ativo"`
}

// PapelEscolhido valida o papel pedido.
func (d AtualizarUsuarioRequestDto) PapelEscolhido() (papel.Papel, error) {
	p, ok := papel.De(d.Papel)
	if !ok {
		return "", ErrPapelInvalido
	}
	return p, nil
}

// AtivoOu devolve o valor pedido ou o atual, quando o campo não veio.
func (d AtualizarUsuarioRequestDto) AtivoOu(atual bool) bool {
	if d.Ativo == nil {
		return atual
	}
	return *d.Ativo
}

// TrocarSenhaRequestDto é o corpo de `PATCH /usuarios/{uuid}/senha`.
type TrocarSenhaRequestDto struct {
	Senha string `json:"senha" binding:"required"`
}
