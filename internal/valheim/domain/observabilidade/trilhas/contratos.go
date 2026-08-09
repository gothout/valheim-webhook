// Package trilhas serve, pela API, as últimas linhas do log do processo — é a
// aba "Logs" do painel.
//
// Ele não tem entidade nem tabela: a fonte é o anel em memória do
// `internal/pkg/log/memoria`, que entra aqui por interface. É um subdomínio de
// domínio sem `model.go` nem `repository.go`, e o motivo é honesto: o dado já
// existe no processo, e persisti-lo no mesmo Postgres de que o processo depende
// seria perder exatamente o log que interessa no dia em que o banco cair.
package trilhas

import (
	"time"

	"valheim-webhook/internal/pkg/log/memoria"
)

// Identidade do subdomínio (observador de erros e logs de boot).
const (
	Sistema    = "valheim"
	Dominio    = "observabilidade"
	Subdominio = "trilhas"
)

// Linha é uma entrada do log.
type Linha struct {
	Seq       int64
	Ts        time.Time
	Nivel     string
	Mensagem  string
	Atributos map[string]string
}

// Filtro é o recorte pedido pelo painel.
type Filtro struct {
	Nivel    string
	Busca    string
	DepoisDe int64
	Limite   int
}

// Fonte é o anel de log visto por uma fresta.
//
// Declarada aqui, no consumidor: o subdomínio não sabe que existe um anel, só
// que alguém sabe responder "as últimas linhas". Nos testes, um dublê responde
// sem que nada precise logar de verdade.
type Fonte interface {
	Ultimas(f memoria.Filtro) []memoria.Linha
	UltimaSequencia() int64
}

// Dependencias é o que o boot liga neste subdomínio.
type Dependencias struct {
	Fonte Fonte
}
