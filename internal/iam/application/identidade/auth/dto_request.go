package auth

// LoginRequestDto é o corpo de `POST /auth/login`.
type LoginRequestDto struct {
	Email string `json:"email" binding:"required"`
	Senha string `json:"senha" binding:"required"`
}

// TrocarMinhaSenhaRequestDto é o corpo de `PATCH /auth/eu/senha`.
//
// A senha ATUAL é exigida mesmo havendo sessão válida: é o que impede que uma
// aba deixada aberta numa máquina emprestada vire uma troca de dono da conta.
type TrocarMinhaSenhaRequestDto struct {
	SenhaAtual string `json:"senha_atual" binding:"required"`
	SenhaNova  string `json:"senha_nova" binding:"required"`
}
