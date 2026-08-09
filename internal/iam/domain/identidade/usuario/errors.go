package usuario

import "errors"

var (
	// ErrNotFound é o usuário inexistente.
	ErrNotFound = errors.New("usuário não encontrado")
	// ErrEmailEmUso é o cadastro repetido.
	ErrEmailEmUso = errors.New("e-mail já cadastrado")
	// ErrEmailInvalido é o e-mail malformado.
	ErrEmailInvalido = errors.New("e-mail inválido")
	// ErrSenhaFraca é a senha fora do tamanho aceito.
	ErrSenhaFraca = errors.New("senha fora do tamanho aceito")
	// ErrPapelInvalido é o papel fora do vocabulário.
	ErrPapelInvalido = errors.New("papel inválido")
	// ErrNomeVazio é o cadastro sem nome.
	ErrNomeVazio = errors.New("nome obrigatório")
	// ErrCredenciais é a recusa do login. É UMA sentinela para os dois casos
	// (e-mail que não existe e senha errada) de propósito: distingui-los
	// contaria a quem tenta adivinhar quais e-mails estão cadastrados.
	ErrCredenciais = errors.New("e-mail ou senha inválidos")
	// ErrContaInativa é o login de conta desligada.
	ErrContaInativa = errors.New("conta inativa")
	// ErrUltimoAdmin barra remover (ou rebaixar, ou desativar) o último
	// administrador: a instalação ficaria sem ninguém capaz de administrá-la, e
	// o conserto seria por SQL.
	ErrUltimoAdmin = errors.New("é o último administrador ativo")
	// ErrAutoRemocao barra o administrador de remover a própria conta.
	ErrAutoRemocao = errors.New("não é possível remover a própria conta")
	// ErrPersistencia embrulha a falha do banco.
	ErrPersistencia = errors.New("falha ao acessar os usuários")
)

// errCodes registra sentinela → código estável para o observador de erros.
var errCodes = map[error]string{
	ErrNotFound:      "ErrNotFound",
	ErrEmailEmUso:    "ErrEmailEmUso",
	ErrEmailInvalido: "ErrEmailInvalido",
	ErrSenhaFraca:    "ErrSenhaFraca",
	ErrPapelInvalido: "ErrPapelInvalido",
	ErrNomeVazio:     "ErrNomeVazio",
	ErrCredenciais:   "ErrCredenciais",
	ErrContaInativa:  "ErrContaInativa",
	ErrUltimoAdmin:   "ErrUltimoAdmin",
	ErrAutoRemocao:   "ErrAutoRemocao",
	ErrPersistencia:  "ErrPersistencia",
}
