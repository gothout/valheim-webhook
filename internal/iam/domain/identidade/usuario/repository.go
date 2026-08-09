package usuario

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository é o acesso a dados do subdomínio.
type Repository interface {
	Create(ctx context.Context, u *Usuario) error
	Update(ctx context.Context, u *Usuario) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByUUID(ctx context.Context, id uuid.UUID) (*Usuario, error)
	FindByEmail(ctx context.Context, email string) (*Usuario, error)
	List(ctx context.Context) ([]Usuario, error)
	// ContarAdminsAtivos é o que sustenta a regra do último administrador.
	// `exceto` tira um usuário da conta (a própria pessoa que está sendo
	// removida ou rebaixada).
	ContarAdminsAtivos(ctx context.Context, exceto uuid.UUID) (int64, error)
	// MarcarAcesso carimba o último login sem reescrever o resto da linha.
	MarcarAcesso(ctx context.Context, id uuid.UUID, quando time.Time) error
	// Total conta os usuários cadastrados (o boot usa para saber se a
	// instalação é nova).
	Total(ctx context.Context) (int64, error)
}

type repositoryImpl struct{ db *gorm.DB }

// NewRepository monta o repositório sobre o pool do processo.
func NewRepository(db *gorm.DB) Repository { return &repositoryImpl{db: db} }

func (r *repositoryImpl) Create(ctx context.Context, u *Usuario) error {
	err := r.db.WithContext(ctx).Create(u).Error
	switch {
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return ErrEmailEmUso
	case err != nil:
		return fmt.Errorf("%w: criar usuário: %v", ErrPersistencia, err)
	}
	return nil
}

func (r *repositoryImpl) Update(ctx context.Context, u *Usuario) error {
	// A atualização é por MAPA, e não pelo struct: com struct, o `Updates` do
	// GORM ignora os campos zerados e `ativo = false` não teria efeito nenhum —
	// desativar um usuário falharia em silêncio, que é o pior modo de falhar
	// numa operação de segurança.
	err := r.db.WithContext(ctx).Model(&Usuario{}).
		Where("uuid = ?", u.UUID).
		Updates(map[string]any{
			"nome":          u.Nome,
			"email":         u.Email,
			"senha_hash":    u.SenhaHash,
			"papel":         string(u.Papel),
			"ativo":         u.Ativo,
			"atualizado_em": time.Now().UTC(),
		}).Error
	switch {
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return ErrEmailEmUso
	case err != nil:
		return fmt.Errorf("%w: atualizar usuário: %v", ErrPersistencia, err)
	}
	return nil
}

func (r *repositoryImpl) Delete(ctx context.Context, id uuid.UUID) error {
	resultado := r.db.WithContext(ctx).Where("uuid = ?", id).Delete(&Usuario{})
	if resultado.Error != nil {
		return fmt.Errorf("%w: remover usuário: %v", ErrPersistencia, resultado.Error)
	}
	if resultado.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repositoryImpl) FindByUUID(ctx context.Context, id uuid.UUID) (*Usuario, error) {
	var u Usuario
	err := r.db.WithContext(ctx).Where("uuid = ?", id).First(&u).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("%w: buscar usuário: %v", ErrPersistencia, err)
	}
	return &u, nil
}

func (r *repositoryImpl) FindByEmail(ctx context.Context, email string) (*Usuario, error) {
	var u Usuario
	// A comparação é insensível a maiúsculas porque o e-mail é a identidade de
	// login: quem se cadastrou como `Thor@casa.com` vai digitar `thor@casa.com`
	// na segunda vez.
	err := r.db.WithContext(ctx).Where("LOWER(email) = LOWER(?)", email).First(&u).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("%w: buscar usuário por e-mail: %v", ErrPersistencia, err)
	}
	return &u, nil
}

func (r *repositoryImpl) List(ctx context.Context) ([]Usuario, error) {
	var usuarios []Usuario
	err := r.db.WithContext(ctx).Order("nome ASC").Find(&usuarios).Error
	if err != nil {
		return nil, fmt.Errorf("%w: listar usuários: %v", ErrPersistencia, err)
	}
	return usuarios, nil
}

func (r *repositoryImpl) ContarAdminsAtivos(ctx context.Context, exceto uuid.UUID) (int64, error) {
	// `string(...)` explícito: o driver recebe um tipo básico, e não um tipo
	// nomeado cuja codificação dependeria de reflexão.
	consulta := r.db.WithContext(ctx).Model(&Usuario{}).
		Where("papel = ? AND ativo = TRUE", string(papelAdmin))
	if exceto != uuid.Nil {
		consulta = consulta.Where("uuid <> ?", exceto)
	}

	var total int64
	if err := consulta.Count(&total).Error; err != nil {
		return 0, fmt.Errorf("%w: contar administradores: %v", ErrPersistencia, err)
	}
	return total, nil
}

func (r *repositoryImpl) MarcarAcesso(ctx context.Context, id uuid.UUID, quando time.Time) error {
	err := r.db.WithContext(ctx).Model(&Usuario{}).
		Where("uuid = ?", id).
		Update("ultimo_acesso_em", quando).Error
	if err != nil {
		return fmt.Errorf("%w: marcar acesso: %v", ErrPersistencia, err)
	}
	return nil
}

func (r *repositoryImpl) Total(ctx context.Context) (int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&Usuario{}).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("%w: contar usuários: %v", ErrPersistencia, err)
	}
	return total, nil
}
