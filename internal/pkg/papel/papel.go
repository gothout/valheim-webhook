// Package papel é o vocabulário de autorização do produto.
//
// Ele existe como pacote próprio, e não como constante dentro do subdomínio de
// usuários, por causa de uma regra de camada: o middleware que EXIGE o papel
// não pode importar o subdomínio que o GUARDA (seria o ciclo clássico —
// controllers importam o middleware). Com o vocabulário num pacote folha, os
// dois lados falam a mesma língua sem se enxergarem.
//
// São dois papéis, e a divisão é entre quem MEXE e quem OLHA:
//
//   - admin: cria e remove usuários, configura a integração com o Discord, lê
//     os logs do processo. É quem administra a instalação;
//   - visualizador: vê o painel, os eventos e os personagens. É o resto do
//     grupo que joga no servidor.
//
// Não há um terceiro nível porque não há uma terceira decisão a tomar: tudo o
// que é destrutivo aqui é administração, e tudo o que não é, é leitura.
package papel

import "strings"

// Papel é o vocabulário fechado.
type Papel string

const (
	// Admin administra a instalação.
	Admin Papel = "admin"
	// Visualizador só lê.
	Visualizador Papel = "visualizador"
)

// Conhecidos lista os papéis válidos (validação de DTO e do painel).
func Conhecidos() []Papel { return []Papel{Admin, Visualizador} }

// Valido diz se o papel pertence ao vocabulário.
func (p Papel) Valido() bool {
	return p == Admin || p == Visualizador
}

// EhAdmin é o teste que o middleware faz.
func (p Papel) EhAdmin() bool { return p == Admin }

// Rotulo é o papel em português, para o painel.
func (p Papel) Rotulo() string {
	switch p {
	case Admin:
		return "Administrador"
	case Visualizador:
		return "Visualizador"
	default:
		return string(p)
	}
}

// De normaliza um texto em Papel. Texto fora do vocabulário devolve `false` —
// nunca um papel silenciosamente inventado.
func De(valor string) (Papel, bool) {
	p := Papel(strings.ToLower(strings.TrimSpace(valor)))
	return p, p.Valido()
}
