package integracao

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository é o acesso a dados do subdomínio.
type Repository interface {
	// Buscar devolve a configuração de um serviço (ErrNotFound quando ainda
	// não foi salva pelo painel).
	Buscar(ctx context.Context, servico string) (*Integracao, error)
	// Salvar grava por UPSERT na chave natural (`servico`).
	Salvar(ctx context.Context, i *Integracao) error
}

// ErrNotFound é a ausência de configuração salva — não é falha: significa
// "ainda vale o que veio do configs.json".
var ErrNotFound = errors.New("configuração não encontrada")

type repositoryImpl struct{ db *gorm.DB }

// NewRepository monta o repositório sobre o pool do processo.
func NewRepository(db *gorm.DB) Repository { return &repositoryImpl{db: db} }

func (r *repositoryImpl) Buscar(ctx context.Context, servico string) (*Integracao, error) {
	var i Integracao
	err := r.db.WithContext(ctx).Where("servico = ?", servico).First(&i).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("%w: buscar integração: %v", ErrPersistencia, err)
	}
	return &i, nil
}

func (r *repositoryImpl) Salvar(ctx context.Context, i *Integracao) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "servico"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"habilitado", "modo", "webhook_url", "bot_token", "canal_id",
			"username", "eventos", "atualizado_por", "atualizado_em",
		}),
	}).Create(i).Error
	if err != nil {
		return fmt.Errorf("%w: gravar integração: %v", ErrPersistencia, err)
	}
	return nil
}
