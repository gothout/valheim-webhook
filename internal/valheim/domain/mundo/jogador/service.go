package jogador

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Transicao diz se o acontecimento MUDOU o estado do personagem.
//
// Existe por causa do Discord. O servidor de Valheim emite `Got character
// ZDOID` também quando alguém renasce depois de morrer — se o receptor
// anunciasse toda ocorrência, o canal receberia "Fulano entrou" três vezes na
// mesma noite. Quem decide notificar é a camada de aplicação, e é este booleano
// que ela consulta.
type Transicao bool

const (
	// Mudou — o personagem estava fora e entrou (ou estava dentro e saiu).
	Mudou Transicao = true
	// Manteve — o acontecimento é real, foi registrado, mas o estado já era
	// esse.
	Manteve Transicao = false
)

// Service concentra as regras do subdomínio.
type Service interface {
	// RegistrarEntrada põe o personagem no mundo.
	RegistrarEntrada(ctx context.Context, servidor, nome, zdoid string, quando time.Time) (*Jogador, Transicao, error)
	// RegistrarSaida tira o personagem do mundo e fecha a sessão.
	RegistrarSaida(ctx context.Context, servidor, nome string, quando time.Time) (*Jogador, Transicao, error)
	// RegistrarSaidaPorZDOID é o caminho da linha de saída, que traz o objeto
	// e não o nome.
	RegistrarSaidaPorZDOID(ctx context.Context, servidor, zdoid string, quando time.Time) (*Jogador, Transicao, error)
	// RegistrarMorte soma uma morte. Não tira ninguém do mundo: morrer no
	// Valheim é renascer na cama.
	RegistrarMorte(ctx context.Context, servidor, nome string, quando time.Time) (*Jogador, error)
	// RegistrarAtividade só empurra o "visto por último" (uma fala, por
	// exemplo).
	RegistrarAtividade(ctx context.Context, servidor, nome string, quando time.Time) (*Jogador, error)
	// ReiniciarServidor derruba todo mundo: o mundo subiu do zero e ninguém
	// está mais lá dentro.
	ReiniciarServidor(ctx context.Context, servidor string, quando time.Time) (int64, error)
	// List devolve os personagens que casam com o filtro.
	List(ctx context.Context, f ListFilter) ([]Jogador, int64, error)
}

type serviceImpl struct{ repo Repository }

// NewService monta o serviço sobre o repositório.
func NewService(repo Repository) Service { return &serviceImpl{repo: repo} }

func (s *serviceImpl) RegistrarEntrada(ctx context.Context, servidor, nome, zdoid string, quando time.Time) (*Jogador, Transicao, error) {
	limpo := strings.TrimSpace(nome)
	if limpo == "" {
		return nil, Manteve, obs.Observe(ctx, ErrNomeVazio)
	}

	j, err := s.carregarOuCriar(ctx, servidor, limpo, quando)
	if err != nil {
		return nil, Manteve, err
	}

	transicao := Manteve
	if !j.Online {
		transicao = Mudou
		j.Online = true
		entrada := quando
		j.EntrouEm = &entrada
		j.Sessoes++
	}
	// O ZDOID é atualizado SEMPRE, mesmo sem transição: no renascimento o
	// personagem ganha um objeto novo, e é o novo que a linha de saída vai
	// citar.
	j.ZDOID = strings.TrimSpace(zdoid)
	j.UltimoEm = quando

	if err := s.repo.Salvar(ctx, j); err != nil {
		return nil, Manteve, obs.Observe(ctx, err)
	}
	return j, transicao, nil
}

func (s *serviceImpl) RegistrarSaida(ctx context.Context, servidor, nome string, quando time.Time) (*Jogador, Transicao, error) {
	limpo := strings.TrimSpace(nome)
	if limpo == "" {
		return nil, Manteve, obs.Observe(ctx, ErrNomeVazio)
	}

	j, err := s.repo.FindByNome(ctx, servidor, limpo)
	if err != nil {
		return nil, Manteve, obs.Observe(ctx, err)
	}
	return s.encerrarSessao(ctx, j, quando)
}

func (s *serviceImpl) RegistrarSaidaPorZDOID(ctx context.Context, servidor, zdoid string, quando time.Time) (*Jogador, Transicao, error) {
	j, err := s.repo.FindByZDOID(ctx, servidor, zdoid)
	if err != nil {
		// Ausência aqui é comum e NÃO é falha: o servidor limpa objetos de
		// sessões anteriores ao reiniciar, e nenhum personagem online é dono
		// deles. Quem chamou trata como "saída de ninguém".
		if errors.Is(err, ErrNotFound) {
			return nil, Manteve, ErrNotFound
		}
		return nil, Manteve, obs.Observe(ctx, err)
	}
	return s.encerrarSessao(ctx, j, quando)
}

func (s *serviceImpl) RegistrarMorte(ctx context.Context, servidor, nome string, quando time.Time) (*Jogador, error) {
	limpo := strings.TrimSpace(nome)
	if limpo == "" {
		return nil, obs.Observe(ctx, ErrNomeVazio)
	}

	j, err := s.carregarOuCriar(ctx, servidor, limpo, quando)
	if err != nil {
		return nil, err
	}
	j.Mortes++
	j.UltimoEm = quando

	if err := s.repo.Salvar(ctx, j); err != nil {
		return nil, obs.Observe(ctx, err)
	}
	return j, nil
}

func (s *serviceImpl) RegistrarAtividade(ctx context.Context, servidor, nome string, quando time.Time) (*Jogador, error) {
	limpo := strings.TrimSpace(nome)
	if limpo == "" {
		return nil, obs.Observe(ctx, ErrNomeVazio)
	}

	j, err := s.carregarOuCriar(ctx, servidor, limpo, quando)
	if err != nil {
		return nil, err
	}
	j.UltimoEm = quando

	if err := s.repo.Salvar(ctx, j); err != nil {
		return nil, obs.Observe(ctx, err)
	}
	return j, nil
}

func (s *serviceImpl) ReiniciarServidor(ctx context.Context, servidor string, quando time.Time) (int64, error) {
	derrubados, err := s.repo.DerrubarTodos(ctx, servidor, quando)
	if err != nil {
		return 0, obs.Observe(ctx, err)
	}
	return derrubados, nil
}

func (s *serviceImpl) List(ctx context.Context, f ListFilter) ([]Jogador, int64, error) {
	jogadores, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, 0, obs.Observe(ctx, err)
	}
	return jogadores, total, nil
}

// encerrarSessao fecha a sessão em curso e soma o tempo.
func (s *serviceImpl) encerrarSessao(ctx context.Context, j *Jogador, quando time.Time) (*Jogador, Transicao, error) {
	transicao := Manteve
	if j.Online {
		transicao = Mudou
		j.Online = false
		if j.EntrouEm != nil {
			// Relógio para trás (carimbo do jogo em fuso diferente, ou máquina
			// acertando a hora) não pode DIMINUIR o tempo somado.
			if duracao := quando.Sub(*j.EntrouEm); duracao > 0 {
				j.TempoTotalSeg += int64(duracao.Seconds())
			}
		}
		j.EntrouEm = nil
		j.ZDOID = ""
	}
	j.UltimoEm = quando

	if err := s.repo.Salvar(ctx, j); err != nil {
		return nil, Manteve, obs.Observe(ctx, err)
	}
	return j, transicao, nil
}

// carregarOuCriar busca o personagem e o cria na primeira aparição.
func (s *serviceImpl) carregarOuCriar(ctx context.Context, servidor, nome string, quando time.Time) (*Jogador, error) {
	j, err := s.repo.FindByNome(ctx, servidor, nome)
	switch {
	case err == nil:
		return j, nil
	case !errors.Is(err, ErrNotFound):
		return nil, obs.Observe(ctx, err)
	}

	return &Jogador{
		UUID:       uuid.New(),
		Servidor:   servidor,
		Nome:       nome,
		PrimeiroEm: quando,
		UltimoEm:   quando,
	}, nil
}
