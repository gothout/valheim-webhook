package jwt

import "errors"

var (
	// ErrNotInitialized é o uso do emissor antes do boot.
	ErrNotInitialized = errors.New("emissor de sessão não inicializado")
	// ErrSegredo é a falha ao obter um segredo de assinatura.
	ErrSegredo = errors.New("falha ao preparar o segredo da sessão")
	// ErrAssinatura é a falha ao assinar um token.
	ErrAssinatura = errors.New("falha ao assinar o token da sessão")
	// ErrTokenAusente é a requisição sem token.
	ErrTokenAusente = errors.New("sessão ausente")
	// ErrTokenInvalido cobre assinatura errada, formato quebrado e emissor
	// desconhecido — uma sentinela só, porque a diferença entre elas só
	// interessaria a quem está tentando forjar.
	ErrTokenInvalido = errors.New("sessão inválida")
	// ErrTokenExpirado é a sessão vencida (o painel manda para o login).
	ErrTokenExpirado = errors.New("sessão expirada")
)
