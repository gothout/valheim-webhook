package evento

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"valheim-webhook/internal/pkg/pagination"
)

// Repository é o acesso a dados do subdomínio.
type Repository interface {
	// Create grava o evento já interpretado.
	Create(ctx context.Context, e *Evento) error
	// FindByUUID devolve um evento pelo identificador (ErrNotFound quando não
	// existe).
	FindByUUID(ctx context.Context, id uuid.UUID) (*Evento, error)
	// List devolve a página e o total que casa com o filtro.
	List(ctx context.Context, f ListFilter, p pagination.Params) ([]Evento, int64, error)
	// Resumir conta os eventos por tipo a partir de `desde` (nulo = tudo).
	Resumir(ctx context.Context, desde *time.Time) (Resumo, error)
	// JogadorPorZDOID devolve o nome do personagem dono de um ZDOID.
	//
	// É a ponte entre a saída e a entrada: a linha que anuncia a desconexão
	// (`Destroying abandoned…`) traz o ZDOID e NÃO traz o nome — o nome só
	// apareceu quando o personagem entrou. Sem esta consulta, todo "saiu"
	// ficaria anônimo.
	JogadorPorZDOID(ctx context.Context, zdoid string) (string, error)
}

type repositoryImpl struct{ db *gorm.DB }

// NewRepository monta o repositório sobre o pool do processo.
func NewRepository(db *gorm.DB) Repository { return &repositoryImpl{db: db} }

func (r *repositoryImpl) Create(ctx context.Context, e *Evento) error {
	if err := r.db.WithContext(ctx).Create(e).Error; err != nil {
		return fmt.Errorf("%w: gravar evento: %v", ErrPersistencia, err)
	}
	return nil
}

func (r *repositoryImpl) FindByUUID(ctx context.Context, id uuid.UUID) (*Evento, error) {
	var e Evento
	err := r.db.WithContext(ctx).Where("uuid = ?", id).First(&e).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("%w: buscar evento: %v", ErrPersistencia, err)
	}
	return &e, nil
}

func (r *repositoryImpl) List(ctx context.Context, f ListFilter, p pagination.Params) ([]Evento, int64, error) {
	// `Session` marca a consulta como REUTILIZÁVEL: sem ela, o GORM considera
	// o encadeamento consumido pelo `Count` e o `Find` seguinte herdaria estado
	// do anterior (o `SELECT count(*)`, entre outros). É o jeito documentado de
	// contar e listar com o mesmo filtro.
	consulta := aplicarFiltro(r.db.WithContext(ctx).Model(&Evento{}), f).
		Session(&gorm.Session{})

	var total int64
	if err := consulta.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("%w: contar eventos: %v", ErrPersistencia, err)
	}
	if total == 0 {
		return []Evento{}, 0, nil
	}

	var eventos []Evento
	// A ordenação tem desempate por `criado_em`: o carimbo do servidor de
	// Valheim tem resolução de SEGUNDO, e uma partida inteira entrando junto
	// produz vários eventos no mesmo segundo. Sem o desempate, a ordem deles
	// mudaria a cada consulta e o feed do painel embaralharia sozinho.
	err := p.Scope(consulta).
		Order("ocorrido_em DESC, criado_em DESC").
		Find(&eventos).Error
	if err != nil {
		return nil, 0, fmt.Errorf("%w: listar eventos: %v", ErrPersistencia, err)
	}
	return eventos, total, nil
}

func (r *repositoryImpl) Resumir(ctx context.Context, desde *time.Time) (Resumo, error) {
	consulta := r.db.WithContext(ctx).Model(&Evento{})
	if desde != nil {
		consulta = consulta.Where("ocorrido_em >= ?", *desde)
	}

	var linhas []struct {
		Tipo  string
		Total int64
	}
	if err := consulta.Select("tipo, count(*) AS total").Group("tipo").Scan(&linhas).Error; err != nil {
		return Resumo{}, fmt.Errorf("%w: resumir eventos: %v", ErrPersistencia, err)
	}

	resumo := Resumo{PorTipo: make(map[Tipo]int64, len(linhas))}
	for _, linha := range linhas {
		resumo.PorTipo[Tipo(linha.Tipo)] = linha.Total
		resumo.Total += linha.Total
	}

	// O último evento sai de uma consulta própria: um MAX() junto do GROUP BY
	// devolveria o máximo POR TIPO, não o do servidor.
	consultaUltimo := r.db.WithContext(ctx).Model(&Evento{})
	if desde != nil {
		consultaUltimo = consultaUltimo.Where("ocorrido_em >= ?", *desde)
	}

	var ultimo sql.NullTime
	if err := consultaUltimo.Select("MAX(ocorrido_em)").Row().Scan(&ultimo); err != nil {
		return Resumo{}, fmt.Errorf("%w: último evento: %v", ErrPersistencia, err)
	}
	if ultimo.Valid {
		quando := ultimo.Time.UTC()
		resumo.UltimoEvento = &quando
	}

	return resumo, nil
}

func (r *repositoryImpl) JogadorPorZDOID(ctx context.Context, zdoid string) (string, error) {
	limpo := strings.TrimSpace(zdoid)
	if limpo == "" {
		return "", nil
	}

	buscar := func(condicao string, valor any) (string, error) {
		var nome string
		err := r.db.WithContext(ctx).Model(&Evento{}).
			Select("jogador").
			Where(condicao, valor).
			Where("jogador <> ''").
			Order("ocorrido_em DESC, criado_em DESC").
			Limit(1).
			Row().Scan(&nome)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return "", nil // ninguém entrou com esse ZDOID: o "saiu" fica anônimo
		case err != nil:
			return "", fmt.Errorf("%w: resolver jogador por zdoid: %v", ErrPersistencia, err)
		}
		return nome, nil
	}

	nome, err := buscar("zdoid = ?", limpo)
	if err != nil || nome != "" {
		return nome, err
	}

	// Sem casamento exato, tenta pelo identificador do dono (a parte antes do
	// `:`). Versões diferentes do servidor incrementam o segundo número entre a
	// entrada e a limpeza do objeto.
	if dono, _, achou := strings.Cut(limpo, ":"); achou && dono != "" {
		return buscar("zdoid LIKE ?", dono+":%")
	}
	return "", nil
}

// aplicarFiltro traduz o ListFilter em cláusulas SQL.
func aplicarFiltro(consulta *gorm.DB, f ListFilter) *gorm.DB {
	if len(f.Tipos) > 0 {
		// Convertido para `[]string`: o driver recebe tipos básicos, e não um
		// tipo nomeado cuja codificação dependeria de reflexão.
		tipos := make([]string, 0, len(f.Tipos))
		for _, tipo := range f.Tipos {
			tipos = append(tipos, string(tipo))
		}
		consulta = consulta.Where("tipo IN ?", tipos)
	}
	if f.Jogador != "" {
		consulta = consulta.Where("jogador = ?", f.Jogador)
	}
	if f.Desde != nil {
		consulta = consulta.Where("ocorrido_em >= ?", *f.Desde)
	}
	if f.Ate != nil {
		consulta = consulta.Where("ocorrido_em <= ?", *f.Ate)
	}
	if f.Busca != "" {
		// ILIKE (busca sem diferenciar maiúsculas) é específico do Postgres, e
		// este repositório é do Postgres — o que o mantém honesto é a interface
		// acima, que não promete portabilidade nenhuma.
		//
		// `%` e `_` do usuário são escapados com `\`, que é o caractere de
		// escape padrão do LIKE no Postgres (não é preciso `ESCAPE`). Sem isso,
		// uma busca por `100%` viraria "qualquer coisa" e varreria a tabela.
		//
		// O parâmetro é NOMEADO porque aparece duas vezes na mesma condição.
		padrao := "%" + escaparLike(f.Busca) + "%"
		consulta = consulta.Where("texto ILIKE @p OR linha ILIKE @p",
			map[string]any{"p": padrao})
	}
	return consulta
}

// escaparLike neutraliza os curingas do LIKE dentro do texto buscado.
func escaparLike(s string) string {
	trocador := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return trocador.Replace(s)
}
