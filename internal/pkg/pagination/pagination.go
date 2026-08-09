// Package pagination padroniza a paginação de todas as listagens da API.
//
// Entrada: `?page=1&pageSize=10`. Saída:
//
//	{"items": [...], "page": 1, "page_size": 10, "total": 137}
//
// Valores fora da faixa são **corrigidos, não recusados**: página menor que 1
// vira 1, tamanho acima do teto vira 200, lixo vira o default. Listar é
// operação segura — devolver 400 porque o cliente mandou `page=0` só cria
// atrito sem proteger nada. O que o teto protege de verdade é o banco: um
// servidor com meses de eventos tem centenas de milhares de linhas, e sem
// `pageSize` máximo um `?pageSize=1000000` varre a tabela inteira.
//
// Pacote folha de `internal/pkg`: importa gin, gorm e stdlib.
package pagination

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Limites da API. O teto é 200 (e não 100, como num ERP) porque o feed do
// painel é uma lista longa e barata: cada linha é um registro pequeno.
const (
	PaginaPadrao  = 1
	TamanhoPadrao = 50
	TamanhoMaximo = 200
)

// Nomes dos query params (contrato da API — camelCase no `pageSize`).
const (
	ParamPagina  = "page"
	ParamTamanho = "pageSize"
)

// Params é a paginação já normalizada: Page >= 1 e 1 <= PageSize <= 200.
// Construir sempre por Novo/Parse/FromQuery — o valor zero não é válido.
type Params struct {
	Page     int `json:"page" form:"page" example:"1"`
	PageSize int `json:"page_size" form:"pageSize" example:"50"`
}

// Novo normaliza page/pageSize para dentro dos limites.
func Novo(page, pageSize int) Params {
	if page < 1 {
		page = PaginaPadrao
	}
	if pageSize < 1 {
		pageSize = TamanhoPadrao
	}
	if pageSize > TamanhoMaximo {
		pageSize = TamanhoMaximo
	}
	return Params{Page: page, PageSize: pageSize}
}

// Parse normaliza os valores em texto (vazio ou inválido → default).
// É a versão pura, sem Gin — usada pelos testes.
func Parse(page, pageSize string) Params {
	return Novo(inteiro(page, PaginaPadrao), inteiro(pageSize, TamanhoPadrao))
}

// FromQuery lê `page`/`pageSize` da query string do request.
func FromQuery(c *gin.Context) Params {
	if c == nil {
		return Novo(PaginaPadrao, TamanhoPadrao)
	}
	return Parse(c.Query(ParamPagina), c.Query(ParamTamanho))
}

// Offset é o deslocamento SQL da página.
func (p Params) Offset() int { return (p.Page - 1) * p.PageSize }

// Limit é o tamanho da página (LIMIT do SQL).
func (p Params) Limit() int { return p.PageSize }

// Scope aplica LIMIT/OFFSET na query — usar SÓ na busca dos itens, nunca na
// contagem total.
func (p Params) Scope(db *gorm.DB) *gorm.DB {
	return db.Offset(p.Offset()).Limit(p.Limit())
}

// TotalPaginas informa quantas páginas existem para um total de registros.
func (p Params) TotalPaginas(total int64) int {
	if total <= 0 {
		return 0
	}
	return int((total + int64(p.PageSize) - 1) / int64(p.PageSize))
}

// inteiro converte texto para int, caindo no padrão quando vazio ou inválido.
func inteiro(s string, padrao int) int {
	if s == "" {
		return padrao
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return padrao
	}
	return n
}
