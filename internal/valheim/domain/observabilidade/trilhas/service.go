package trilhas

import (
	"context"
	"strings"

	"valheim-webhook/internal/pkg/log/memoria"
)

// LimitePadrao é quantas linhas a tela carrega quando não pede um número.
const LimitePadrao = 200

// LimiteMaximo é o teto — o anel inteiro é pequeno, e devolver tudo a cada
// atualização de tela seria desperdício de banda por nada.
const LimiteMaximo = 500

// NiveisAceitos é o vocabulário do filtro.
func NiveisAceitos() []string { return []string{"DEBUG", "INFO", "WARN", "ERROR"} }

// Service concentra as regras do subdomínio.
type Service interface {
	// Listar devolve as linhas do recorte e a sequência mais recente do anel.
	Listar(ctx context.Context, f Filtro) ([]Linha, int64, error)
}

type serviceImpl struct{ fonte Fonte }

// NewService monta o serviço sobre a fonte.
func NewService(deps Dependencias) Service { return &serviceImpl{fonte: deps.Fonte} }

func (s *serviceImpl) Listar(ctx context.Context, f Filtro) ([]Linha, int64, error) {
	if s.fonte == nil {
		return nil, 0, obs.Observe(ctx, ErrFonteIndisponivel)
	}

	nivel := strings.ToUpper(strings.TrimSpace(f.Nivel))
	if nivel != "" && !nivelConhecido(nivel) {
		return nil, 0, obs.Observe(ctx, ErrNivelInvalido)
	}

	limite := f.Limite
	if limite <= 0 {
		limite = LimitePadrao
	}
	if limite > LimiteMaximo {
		limite = LimiteMaximo
	}

	brutas := s.fonte.Ultimas(memoria.Filtro{
		Nivel:    nivel,
		Busca:    f.Busca,
		DepoisDe: f.DepoisDe,
		Limite:   limite,
	})

	linhas := make([]Linha, 0, len(brutas))
	for _, b := range brutas {
		linhas = append(linhas, Linha{
			Seq:       b.Seq,
			Ts:        b.Ts,
			Nivel:     b.Nivel,
			Mensagem:  b.Mensagem,
			Atributos: b.Atributos,
		})
	}
	return linhas, s.fonte.UltimaSequencia(), nil
}

// nivelConhecido confere o filtro contra o vocabulário.
func nivelConhecido(nivel string) bool {
	for _, aceito := range NiveisAceitos() {
		if nivel == aceito {
			return true
		}
	}
	return false
}
