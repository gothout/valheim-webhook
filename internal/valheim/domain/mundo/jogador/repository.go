package jogador

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository é o acesso a dados do subdomínio.
type Repository interface {
	// FindByNome devolve o personagem (ErrNotFound quando não existe).
	FindByNome(ctx context.Context, servidor, nome string) (*Jogador, error)
	// FindByZDOID devolve o personagem ONLINE dono do ZDOID.
	FindByZDOID(ctx context.Context, servidor, zdoid string) (*Jogador, error)
	// Salvar grava o personagem (insere ou atualiza pela chave natural).
	Salvar(ctx context.Context, j *Jogador) error
	// List devolve os personagens que casam com o filtro, com o total.
	List(ctx context.Context, f ListFilter) ([]Jogador, int64, error)
	// DerrubarTodos marca todo mundo como offline e encerra as sessões abertas,
	// somando o tempo decorrido. Devolve quantos estavam online.
	DerrubarTodos(ctx context.Context, servidor string, quando time.Time) (int64, error)
}

type repositoryImpl struct{ db *gorm.DB }

// NewRepository monta o repositório sobre o pool do processo.
func NewRepository(db *gorm.DB) Repository { return &repositoryImpl{db: db} }

func (r *repositoryImpl) FindByNome(ctx context.Context, servidor, nome string) (*Jogador, error) {
	var j Jogador
	err := r.db.WithContext(ctx).
		Where("servidor = ? AND nome = ?", servidor, nome).
		First(&j).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("%w: buscar personagem: %v", ErrPersistencia, err)
	}
	return &j, nil
}

func (r *repositoryImpl) FindByZDOID(ctx context.Context, servidor, zdoid string) (*Jogador, error) {
	limpo := strings.TrimSpace(zdoid)
	if limpo == "" {
		return nil, ErrNotFound
	}

	var j Jogador
	err := r.db.WithContext(ctx).
		Where("servidor = ? AND zdoid = ? AND online = TRUE", servidor, limpo).
		Order("ultimo_em DESC").
		First(&j).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("%w: buscar personagem por zdoid: %v", ErrPersistencia, err)
	}
	return &j, nil
}

// Salvar grava por UPSERT na chave natural `(servidor, nome)`.
//
// Um `Save` do GORM faria UPDATE e, sem linha afetada, um SELECT e um INSERT —
// três idas ao banco e uma janela entre elas. Aqui é um comando só: o primeiro
// evento de um personagem insere, os seguintes atualizam, e duas linhas de log
// chegando ao mesmo tempo não conseguem criar o mesmo personagem duas vezes
// (quem perde a corrida cai no `DO UPDATE`).
//
// A lista de colunas é explícita de propósito: `uuid`, `criado_em` e
// `primeiro_em` NÃO podem ser sobrescritos — são o que a linha já tem de mais
// antigo, e o registro em memória traz valores novos que perderiam essa
// história.
func (r *repositoryImpl) Salvar(ctx context.Context, j *Jogador) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "servidor"}, {Name: "nome"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"online", "zdoid", "entrou_em", "ultimo_em",
			"sessoes", "mortes", "tempo_total_seg", "atualizado_em",
		}),
	}).Create(j).Error
	if err != nil {
		return fmt.Errorf("%w: gravar personagem: %v", ErrPersistencia, err)
	}
	return nil
}

func (r *repositoryImpl) List(ctx context.Context, f ListFilter) ([]Jogador, int64, error) {
	consulta := r.db.WithContext(ctx).Model(&Jogador{})
	if f.Servidor != "" {
		consulta = consulta.Where("servidor = ?", f.Servidor)
	}
	if f.ApenasOnline {
		consulta = consulta.Where("online = TRUE")
	}
	if f.Nome != "" {
		consulta = consulta.Where("nome ILIKE ?", "%"+escaparLike(f.Nome)+"%")
	}
	// `Session` marca a consulta como reutilizável entre o `Count` e o `Find`
	// (ver o comentário equivalente no repositório de eventos).
	consulta = consulta.Session(&gorm.Session{})

	var total int64
	if err := consulta.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("%w: contar personagens: %v", ErrPersistencia, err)
	}

	var jogadores []Jogador
	// Online primeiro, depois quem apareceu mais recentemente: é a ordem em que
	// o painel quer mostrar, e é barata o bastante para não valer paginação —
	// um servidor caseiro tem dezenas de personagens, não milhares.
	err := consulta.Order("online DESC, ultimo_em DESC").Find(&jogadores).Error
	if err != nil {
		return nil, 0, fmt.Errorf("%w: listar personagens: %v", ErrPersistencia, err)
	}
	return jogadores, total, nil
}

func (r *repositoryImpl) DerrubarTodos(ctx context.Context, servidor string, quando time.Time) (int64, error) {
	// A soma do tempo é feita EM SQL, e não lendo/gravando linha a linha, por
	// um motivo de correção: o reinício do servidor pode pegar dez pessoas
	// online, e dez idas ao banco entre a leitura e a escrita são dez janelas
	// para um evento novo chegar no meio.
	resultado := r.db.WithContext(ctx).Model(&Jogador{}).
		Where("servidor = ? AND online = TRUE", servidor).
		Updates(map[string]any{
			"online": false,
			"zdoid":  "",
			"tempo_total_seg": gorm.Expr(
				"tempo_total_seg + GREATEST(0, EXTRACT(EPOCH FROM (?::timestamptz - COALESCE(entrou_em, ?::timestamptz)))::bigint)",
				quando, quando),
			"entrou_em":     nil,
			"ultimo_em":     quando,
			"atualizado_em": quando,
		})
	if resultado.Error != nil {
		return 0, fmt.Errorf("%w: derrubar personagens: %v", ErrPersistencia, resultado.Error)
	}
	return resultado.RowsAffected, nil
}

// escaparLike neutraliza os curingas do LIKE dentro do texto buscado.
func escaparLike(s string) string {
	trocador := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return trocador.Replace(s)
}
