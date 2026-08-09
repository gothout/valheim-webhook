package middleware

import "errors"

var (
	// ErrNotInitialized é o uso do middleware antes do boot. Ele NUNCA vira
	// rota aberta: cadeia não inicializada responde 403.
	ErrNotInitialized = errors.New("middleware de autorização não inicializado")
	// ErrSessaoAusente é a requisição sem cookie e sem header.
	ErrSessaoAusente = errors.New("sessão ausente")
	// ErrSessaoInvalida é o token que não confere.
	ErrSessaoInvalida = errors.New("sessão inválida")
	// ErrSessaoExpirada é o token vencido — o painel manda para o login.
	ErrSessaoExpirada = errors.New("sessão expirada")
	// ErrUsuarioDesconhecido é o token de uma conta que não existe mais.
	ErrUsuarioDesconhecido = errors.New("usuário da sessão não existe mais")
	// ErrContaInativa é o token de uma conta desligada.
	ErrContaInativa = errors.New("conta inativa")
	// ErrSemPermissao é a rota que exige administrador.
	ErrSemPermissao = errors.New("permissão insuficiente")
)

// errCodes registra sentinela → código estável para o observador de erros.
var errCodes = map[error]string{
	ErrNotInitialized:      "ErrNotInitialized",
	ErrSessaoAusente:       "ErrSessaoAusente",
	ErrSessaoInvalida:      "ErrSessaoInvalida",
	ErrSessaoExpirada:      "ErrSessaoExpirada",
	ErrUsuarioDesconhecido: "ErrUsuarioDesconhecido",
	ErrContaInativa:        "ErrContaInativa",
	ErrSemPermissao:        "ErrSemPermissao",
}
