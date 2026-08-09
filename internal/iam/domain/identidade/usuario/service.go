package usuario

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"valheim-webhook/internal/pkg/papel"
)

// CustoDoHash é o custo do bcrypt.
//
// 12 é acima do padrão da biblioteca (10) e leva algumas centenas de
// milissegundos numa máquina caseira — o que é aceitável para uma operação que
// acontece no login, e caro o bastante para quem tentar adivinhar senhas em
// lote depois de levar o banco embora.
const CustoDoHash = 12

// hashDeReferencia é um bcrypt VÁLIDO de uma senha pública e conhecida
// ("password", custo 10).
//
// Ele serve para o login gastar tempo comparável quando o e-mail não existe e
// quando a senha está errada. Sem isto, a resposta volta na hora para e-mail
// inexistente e demora o custo do bcrypt para e-mail existente — e a diferença
// é medível pela rede, o que transforma a tela de login num verificador de
// quais e-mails estão cadastrados.
//
// Precisa ser um hash SINTATICAMENTE VÁLIDO: um valor inventado faria o bcrypt
// recusar o formato antes de fazer conta nenhuma, e a comparação voltaria
// instantânea — desfazendo em silêncio a proteção que ela existe para dar.
const hashDeReferencia = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// Service concentra as regras do subdomínio.
type Service interface {
	Criar(ctx context.Context, nome, email, senha string, p papel.Papel) (*Usuario, error)
	Listar(ctx context.Context) ([]Usuario, error)
	Ler(ctx context.Context, id uuid.UUID) (*Usuario, error)
	// Atualizar muda nome, papel e situação. Senha tem caminho próprio.
	Atualizar(ctx context.Context, id uuid.UUID, nome string, p papel.Papel, ativo bool) (*Usuario, error)
	// TrocarSenha define uma senha nova (usada pelo próprio dono e pelo admin).
	TrocarSenha(ctx context.Context, id uuid.UUID, senha string) error
	Remover(ctx context.Context, id, autor uuid.UUID) error
	// Autenticar confere e-mail e senha.
	Autenticar(ctx context.Context, email, senha string) (*Usuario, error)
	// RegistrarAcesso carimba o login bem-sucedido.
	RegistrarAcesso(ctx context.Context, id uuid.UUID) error
	// GarantirAdministrador cria a conta inicial quando a instalação é nova.
	GarantirAdministrador(ctx context.Context, nome, email, senha string) (*Usuario, string, error)
}

type serviceImpl struct{ repo Repository }

// NewService monta o serviço sobre o repositório.
func NewService(repo Repository) Service { return &serviceImpl{repo: repo} }

func (s *serviceImpl) Criar(ctx context.Context, nome, email, senha string, p papel.Papel) (*Usuario, error) {
	nome, email, err := s.validarIdentificacao(ctx, nome, email)
	if err != nil {
		return nil, err
	}
	if !p.Valido() {
		return nil, obs.Observe(ctx, ErrPapelInvalido)
	}
	hash, err := s.hashDaSenha(ctx, senha)
	if err != nil {
		return nil, err
	}

	// A checagem de e-mail repetido é feita ANTES por causa da mensagem (o
	// painel quer dizer "já existe" no campo certo), mas quem garante a
	// unicidade é o índice do banco: entre a consulta e a inserção cabe outro
	// cadastro, e o `ErrEmailEmUso` traduzido do erro de chave é o que fecha
	// essa fresta.
	if _, err := s.repo.FindByEmail(ctx, email); err == nil {
		return nil, obs.Observe(ctx, ErrEmailEmUso)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, obs.Observe(ctx, err)
	}

	u := &Usuario{
		UUID:      uuid.New(),
		Nome:      nome,
		Email:     email,
		SenhaHash: hash,
		Papel:     p,
		Ativo:     true,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, obs.Observe(ctx, err)
	}
	return u, nil
}

func (s *serviceImpl) Listar(ctx context.Context) ([]Usuario, error) {
	usuarios, err := s.repo.List(ctx)
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}
	return usuarios, nil
}

func (s *serviceImpl) Ler(ctx context.Context, id uuid.UUID) (*Usuario, error) {
	u, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}
	return u, nil
}

func (s *serviceImpl) Atualizar(ctx context.Context, id uuid.UUID, nome string, p papel.Papel, ativo bool) (*Usuario, error) {
	u, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}

	nome = strings.TrimSpace(nome)
	if nome == "" {
		return nil, obs.Observe(ctx, ErrNomeVazio)
	}
	if !p.Valido() {
		return nil, obs.Observe(ctx, ErrPapelInvalido)
	}

	// Rebaixar ou desativar o último administrador ativo deixaria a instalação
	// sem ninguém capaz de administrá-la — e o conserto seria por SQL.
	perdeuOPoder := u.Papel.EhAdmin() && (!p.EhAdmin() || !ativo)
	if perdeuOPoder {
		if err := s.exigirOutroAdministrador(ctx, u.UUID); err != nil {
			return nil, err
		}
	}

	u.Nome, u.Papel, u.Ativo = nome, p, ativo
	if err := s.repo.Update(ctx, u); err != nil {
		return nil, obs.Observe(ctx, err)
	}
	return u, nil
}

func (s *serviceImpl) TrocarSenha(ctx context.Context, id uuid.UUID, senha string) error {
	u, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		return obs.Observe(ctx, err)
	}
	hash, err := s.hashDaSenha(ctx, senha)
	if err != nil {
		return err
	}

	u.SenhaHash = hash
	if err := s.repo.Update(ctx, u); err != nil {
		return obs.Observe(ctx, err)
	}
	return nil
}

func (s *serviceImpl) Remover(ctx context.Context, id, autor uuid.UUID) error {
	if id == autor {
		return obs.Observe(ctx, ErrAutoRemocao)
	}

	u, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		return obs.Observe(ctx, err)
	}
	if u.Papel.EhAdmin() && u.Ativo {
		if err := s.exigirOutroAdministrador(ctx, u.UUID); err != nil {
			return err
		}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return obs.Observe(ctx, err)
	}
	return nil
}

func (s *serviceImpl) Autenticar(ctx context.Context, email, senha string) (*Usuario, error) {
	limpo := strings.ToLower(strings.TrimSpace(email))

	u, err := s.repo.FindByEmail(ctx, limpo)
	if errors.Is(err, ErrNotFound) {
		// Gasta o mesmo tempo do caminho de sucesso antes de recusar.
		_ = bcrypt.CompareHashAndPassword([]byte(hashDeReferencia), []byte(senha))
		return nil, obs.Observe(ctx, ErrCredenciais)
	}
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.SenhaHash), []byte(senha)); err != nil {
		return nil, obs.Observe(ctx, ErrCredenciais)
	}
	// A conta inativa é conferida DEPOIS da senha: responder "conta inativa"
	// para quem errou a senha confirmaria que o e-mail existe.
	if !u.PodeEntrar() {
		return nil, obs.Observe(ctx, ErrContaInativa)
	}
	return u, nil
}

func (s *serviceImpl) RegistrarAcesso(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.MarcarAcesso(ctx, id, time.Now().UTC()); err != nil {
		return obs.Observe(ctx, err)
	}
	return nil
}

// GarantirAdministrador cria a conta inicial quando não há usuário nenhum.
//
// Devolve o usuário criado e a senha em texto claro APENAS quando ela foi
// sorteada aqui — é a única vez que ela existe fora do hash, e o boot a imprime
// no log uma vez. Instalação que já tem usuários não é tocada: devolve
// `(nil, "", nil)`.
func (s *serviceImpl) GarantirAdministrador(ctx context.Context, nome, email, senha string) (*Usuario, string, error) {
	total, err := s.repo.Total(ctx)
	if err != nil {
		return nil, "", obs.Observe(ctx, err)
	}
	if total > 0 {
		return nil, "", nil
	}

	sorteada := ""
	if strings.TrimSpace(senha) == "" {
		gerada, err := senhaAleatoria()
		if err != nil {
			return nil, "", obs.Observe(ctx, err)
		}
		senha, sorteada = gerada, gerada
	}

	u, err := s.Criar(ctx, nome, email, senha, papel.Admin)
	if err != nil {
		return nil, "", err
	}
	slog.Info("[USUARIO] conta administrativa inicial criada", "email", u.Email)
	return u, sorteada, nil
}

// exigirOutroAdministrador recusa a operação quando não sobraria nenhum admin.
func (s *serviceImpl) exigirOutroAdministrador(ctx context.Context, exceto uuid.UUID) error {
	restantes, err := s.repo.ContarAdminsAtivos(ctx, exceto)
	if err != nil {
		return obs.Observe(ctx, err)
	}
	if restantes == 0 {
		return obs.Observe(ctx, ErrUltimoAdmin)
	}
	return nil
}

// validarIdentificacao normaliza e confere nome e e-mail.
func (s *serviceImpl) validarIdentificacao(ctx context.Context, nome, email string) (string, string, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return "", "", obs.Observe(ctx, ErrNomeVazio)
	}

	limpo := strings.ToLower(strings.TrimSpace(email))
	endereco, err := mail.ParseAddress(limpo)
	if err != nil || endereco.Address != limpo {
		return "", "", obs.Observe(ctx, ErrEmailInvalido)
	}
	return nome, limpo, nil
}

// hashDaSenha valida o tamanho e devolve o bcrypt.
func (s *serviceImpl) hashDaSenha(ctx context.Context, senha string) (string, error) {
	if len(senha) < MinimoDaSenha || len(senha) > MaximoDaSenha {
		return "", obs.Observe(ctx, ErrSenhaFraca)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(senha), CustoDoHash)
	if err != nil {
		return "", obs.Observe(ctx, err)
	}
	return string(hash), nil
}

// senhaAleatoria gera a senha inicial do administrador.
func senhaAleatoria() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
