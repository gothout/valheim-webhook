package pagination

// Response é o corpo padrão de toda listagem.
//
// Genérico no tipo do item para o controller não perder a tipagem do DTO:
//
//	c.JSON(http.StatusOK, pagination.NovaResponse(dtos, params, total))
type Response[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page" example:"1"`
	PageSize int   `json:"page_size" example:"50"`
	Total    int64 `json:"total" example:"137"`
}

// NovaResponse monta o corpo da listagem.
//
// Lista nula vira lista vazia de propósito: o JSON precisa sair como
// `"items": []`, nunca `"items": null` — o painel faz `.map` no resultado e
// quebraria com null.
func NovaResponse[T any](items []T, p Params, total int64) Response[T] {
	if items == nil {
		items = []T{}
	}
	return Response[T]{Items: items, Page: p.Page, PageSize: p.PageSize, Total: total}
}
