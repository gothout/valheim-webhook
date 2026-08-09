package evento

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"valheim-webhook/internal/pkg/pagination"
)

// MaxLinha é o teto de uma linha de log.
//
// Linha de log de jogo tem centenas de bytes. Oito mil é folga de uma ordem de
// grandeza — e é a diferença entre "guardo o que o servidor disse" e "guardo o
// que qualquer um mandar no `POST /eventos`".
const MaxLinha = 8 * 1024

// Service concentra as regras do subdomínio.
type Service interface {
	// Registrar interpreta uma linha de log e a grava.
	Registrar(ctx context.Context, servidor, linha string) (*Evento, error)
	// Read devolve um evento pelo identificador.
	Read(ctx context.Context, id uuid.UUID) (*Evento, error)
	// List devolve a página que casa com o filtro.
	List(ctx context.Context, f ListFilter, p pagination.Params) ([]Evento, int64, error)
	// Resumir conta os eventos por tipo desde um instante (nulo = tudo).
	Resumir(ctx context.Context, desde *time.Time) (Resumo, error)
}

type serviceImpl struct {
	repo  Repository
	agora func() time.Time
}

// NewService monta o serviço sobre o repositório.
func NewService(repo Repository) Service {
	return &serviceImpl{repo: repo, agora: func() time.Time { return time.Now().UTC() }}
}

// Registrar é o caminho por onde TUDO entra.
//
// A ordem importa: primeiro a linha é validada e interpretada (nada de I/O),
// depois — e só quando o evento é uma saída — o banco é consultado para
// descobrir de quem era o ZDOID, e por fim o registro é gravado.
//
// A resolução do nome é BEST-EFFORT: se a consulta falhar, o evento é gravado
// mesmo assim, sem o nome. Perder o "quem" é ruim; perder o evento é pior.
func (s *serviceImpl) Registrar(ctx context.Context, servidor, linha string) (*Evento, error) {
	bruta := strings.TrimSpace(linha)
	if bruta == "" {
		return nil, obs.Observe(ctx, ErrLinhaVazia)
	}
	if len(bruta) > MaxLinha {
		return nil, obs.Observe(ctx, ErrLinhaLonga)
	}

	analise, err := Analisar(bruta)
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}

	ocorridoEm := s.agora()
	if analise.TemHora {
		ocorridoEm = analise.OcorridoEm
	}

	jogador := analise.Jogador
	if jogador == "" && analise.Tipo == TipoSaiu {
		if nome, err := s.repo.JogadorPorZDOID(ctx, analise.ZDOID); err == nil {
			jogador = nome
		} else {
			// Observado, não propagado: é degradação, não falha da operação.
			_ = obs.Observe(ctx, err)
		}
	}

	evento := &Evento{
		UUID:       uuid.New(),
		Servidor:   strings.TrimSpace(servidor),
		Tipo:       analise.Tipo,
		Jogador:    jogador,
		SteamID:    analise.SteamID,
		ZDOID:      analise.ZDOID,
		Texto:      analise.Texto,
		Linha:      bruta,
		OcorridoEm: ocorridoEm,
	}

	if err := s.repo.Create(ctx, evento); err != nil {
		return nil, obs.Observe(ctx, err)
	}
	return evento, nil
}

func (s *serviceImpl) Read(ctx context.Context, id uuid.UUID) (*Evento, error) {
	e, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		return nil, obs.Observe(ctx, err)
	}
	return e, nil
}

func (s *serviceImpl) List(ctx context.Context, f ListFilter, p pagination.Params) ([]Evento, int64, error) {
	eventos, total, err := s.repo.List(ctx, f, p)
	if err != nil {
		return nil, 0, obs.Observe(ctx, err)
	}
	return eventos, total, nil
}

func (s *serviceImpl) Resumir(ctx context.Context, desde *time.Time) (Resumo, error) {
	resumo, err := s.repo.Resumir(ctx, desde)
	if err != nil {
		return Resumo{}, obs.Observe(ctx, err)
	}
	if resumo.PorTipo == nil {
		resumo.PorTipo = map[Tipo]int64{}
	}
	return resumo, nil
}
