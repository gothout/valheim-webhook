// Package auth é o caso de uso de sessão: entrar, sair, ver quem sou e trocar
// a própria senha.
//
// Ele mora na camada de APLICAÇÃO, e não junto do subdomínio de usuários, por
// uma razão de recorte: login costura DOIS lados — a conta (que valida a senha)
// e o emissor de token (que é infraestrutura) — e nenhum dos dois deve conhecer
// o outro. Aqui, os dois entram por interface (`contratos.go`) e o
// `cmd/bootstrap` faz a ligação.
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Sessao é o que o login produz.
type Sessao struct {
	Token    string
	ExpiraEm time.Time
	Conta    Conta
}

// Service concentra as regras do caso de uso.
type Service interface {
	// Entrar valida as credenciais e devolve a sessão assinada.
	Entrar(ctx context.Context, email, senha string) (*Sessao, error)
	// Eu devolve a conta da sessão em curso.
	Eu(ctx context.Context, id uuid.UUID) (*Conta, error)
	// TrocarMinhaSenha exige a senha atual antes de aceitar a nova.
	TrocarMinhaSenha(ctx context.Context, id uuid.UUID, atual, nova string) error
}

type serviceImpl struct {
	usuarios Usuarios
	emissor  Emissor
}

// NewService monta o serviço sobre as dependências ligadas pelo boot.
func NewService(deps Dependencias) Service {
	return &serviceImpl{usuarios: deps.Usuarios, emissor: deps.Emissor}
}

func (s *serviceImpl) Entrar(ctx context.Context, email, senha string) (*Sessao, error) {
	conta, err := s.usuarios.Autenticar(ctx, email, senha)
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}

	token, expira, err := s.emissor.Emitir(conta.UUID, conta.Nome, conta.Email, string(conta.Papel))
	if err != nil {
		return nil, obs.Observe(ctx, errors.Join(ErrEmissao, err))
	}

	// O carimbo do último acesso é efeito colateral do login, não parte dele:
	// falhar aqui não pode impedir alguém de entrar.
	if err := s.usuarios.RegistrarAcesso(ctx, conta.UUID); err != nil {
		_ = obs.Observe(ctx, err)
	}

	return &Sessao{Token: token, ExpiraEm: expira, Conta: conta}, nil
}

func (s *serviceImpl) Eu(ctx context.Context, id uuid.UUID) (*Conta, error) {
	if id == uuid.Nil {
		return nil, obs.Observe(ctx, ErrSemSessao)
	}
	conta, err := s.usuarios.Ler(ctx, id)
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}
	return &conta, nil
}

func (s *serviceImpl) TrocarMinhaSenha(ctx context.Context, id uuid.UUID, atual, nova string) error {
	if id == uuid.Nil {
		return obs.Observe(ctx, ErrSemSessao)
	}

	conta, err := s.usuarios.Ler(ctx, id)
	if err != nil {
		return obs.Observe(ctx, err)
	}
	// Confere a senha atual pelo mesmo caminho do login — inclusive o custo em
	// tempo, que é o que torna inútil tentar adivinhá-la por aqui.
	if _, err := s.usuarios.Autenticar(ctx, conta.Email, atual); err != nil {
		return obs.Observe(ctx, ErrCredenciais)
	}

	if err := s.usuarios.TrocarSenha(ctx, id, nova); err != nil {
		return obs.Observe(ctx, err)
	}
	return nil
}
