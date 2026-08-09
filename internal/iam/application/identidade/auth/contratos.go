package auth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"valheim-webhook/internal/pkg/papel"
)

// Conta é o mínimo que este caso de uso precisa saber de um usuário.
//
// É um tipo DESTE pacote, e não a entidade do subdomínio de usuários: o que
// atravessa a fronteira é o necessário para montar a sessão — nunca o hash da
// senha, que não tem por que existir fora de lá.
type Conta struct {
	UUID           uuid.UUID
	Nome           string
	Email          string
	Papel          papel.Papel
	Ativo          bool
	UltimoAcessoEm *time.Time
}

// Usuarios é o vizinho `iam/domain/identidade/usuario`, visto por uma fresta.
//
// A interface é declarada aqui, no consumidor, e ligada por um adaptador no
// `cmd/bootstrap` que traduz o vocabulário de erro do outro lado. É o que
// permite testar o login sem banco e sem singleton.
type Usuarios interface {
	// Autenticar confere e-mail e senha (ErrCredenciais / ErrContaInativa).
	Autenticar(ctx context.Context, email, senha string) (Conta, error)
	// Ler devolve a conta pelo identificador (ErrContaDesconhecida).
	Ler(ctx context.Context, id uuid.UUID) (Conta, error)
	// RegistrarAcesso carimba o login bem-sucedido.
	RegistrarAcesso(ctx context.Context, id uuid.UUID) error
	// TrocarSenha define a senha nova.
	TrocarSenha(ctx context.Context, id uuid.UUID, senha string) error
}

// Emissor assina o token da sessão (implementado pelo `infra/jwt`).
type Emissor interface {
	Emitir(usuarioUUID uuid.UUID, nome, email, papel string) (string, time.Time, error)
}

// Dependencias é o que o boot liga neste subdomínio.
type Dependencias struct {
	Usuarios Usuarios
	Emissor  Emissor
}

// Opcoes são as decisões de transporte da sessão.
type Opcoes struct {
	// CookieSeguro marca o cookie como `Secure`. Ver o comentário em
	// config.Security.
	CookieSeguro bool
	// CookieDominio é o `Domain` do cookie. Vazio = só o host que respondeu,
	// que é o mais restrito e o padrão. `.atila.cloud` faz a sessão valer em
	// todos os subdomínios.
	CookieDominio string
}
