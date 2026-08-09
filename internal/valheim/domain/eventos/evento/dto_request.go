package evento

import (
	"strings"
	"time"
)

// ListarRequestDto são os filtros da listagem, lidos da query string.
//
// Tudo é opcional: `GET /eventos` sem query devolve a página mais recente, que
// é o que o painel pede ao abrir.
type ListarRequestDto struct {
	// Tipos aceita repetição (`?tipo=entrou&tipo=saiu`) ou lista separada por
	// vírgula (`?tipo=entrou,saiu`) — as duas formas aparecem em cliente HTTP
	// escrito à mão, e recusar uma delas só rende suporte.
	Tipos []string `form:"tipo"`
	// Jogador casa com o nome exato do personagem.
	Jogador string `form:"jogador"`
	// Desde e Ate são datas RFC3339 (`2026-08-09T00:00:00Z`).
	Desde string `form:"desde"`
	Ate   string `form:"ate"`
	// Busca é um pedaço de texto procurado na fala e na linha crua.
	Busca string `form:"busca"`
}

// ParaFiltro valida e converte o DTO no filtro do repositório.
func (d ListarRequestDto) ParaFiltro() (ListFilter, error) {
	f := ListFilter{
		Jogador: strings.TrimSpace(d.Jogador),
		Busca:   strings.TrimSpace(d.Busca),
	}

	for _, bruto := range d.Tipos {
		for _, pedaco := range strings.Split(bruto, ",") {
			nome := strings.ToLower(strings.TrimSpace(pedaco))
			if nome == "" {
				continue
			}
			tipo := Tipo(nome)
			if !tipo.Valido() {
				return ListFilter{}, ErrTipoInvalido
			}
			f.Tipos = append(f.Tipos, tipo)
		}
	}

	desde, err := instante(d.Desde)
	if err != nil {
		return ListFilter{}, err
	}
	ate, err := instante(d.Ate)
	if err != nil {
		return ListFilter{}, err
	}
	f.Desde, f.Ate = desde, ate

	return f, nil
}

// instante lê uma data RFC3339 opcional.
func instante(valor string) (*time.Time, error) {
	limpo := strings.TrimSpace(valor)
	if limpo == "" {
		return nil, nil
	}
	quando, err := time.Parse(time.RFC3339, limpo)
	if err != nil {
		return nil, ErrDataInvalida
	}
	utc := quando.UTC()
	return &utc, nil
}
