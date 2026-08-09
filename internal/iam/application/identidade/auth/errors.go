package auth

import "errors"

var (
	// ErrCredenciais é a recusa do login — uma só para e-mail inexistente e
	// senha errada.
	ErrCredenciais = errors.New("e-mail ou senha inválidos")
	// ErrContaInativa é o login de conta desligada.
	ErrContaInativa = errors.New("conta inativa")
	// ErrContaDesconhecida é a sessão de uma conta que não existe mais.
	ErrContaDesconhecida = errors.New("conta não encontrada")
	// ErrSenhaFraca é a senha nova fora do tamanho aceito.
	ErrSenhaFraca = errors.New("senha fora do tamanho aceito")
	// ErrSemSessao é o uso das rotas de "eu" sem sessão (defesa em
	// profundidade: o middleware já barraria).
	ErrSemSessao = errors.New("sessão ausente")
	// ErrEmissao é a falha ao assinar o token.
	ErrEmissao = errors.New("falha ao criar a sessão")
	// ErrIndisponivel é a falha do vizinho (banco fora do ar, por exemplo).
	ErrIndisponivel = errors.New("não foi possível concluir agora")
)

// errCodes registra sentinela → código estável para o observador de erros.
var errCodes = map[error]string{
	ErrCredenciais:       "ErrCredenciais",
	ErrContaInativa:      "ErrContaInativa",
	ErrContaDesconhecida: "ErrContaDesconhecida",
	ErrSenhaFraca:        "ErrSenhaFraca",
	ErrSemSessao:         "ErrSemSessao",
	ErrEmissao:           "ErrEmissao",
	ErrIndisponivel:      "ErrIndisponivel",
}
