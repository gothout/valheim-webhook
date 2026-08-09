package middleware

import (
	"context"

	"github.com/google/uuid"

	"valheim-webhook/internal/pkg/papel"
)

// Autor é quem o token diz que está fazendo o request.
type Autor struct {
	UUID  uuid.UUID
	Nome  string
	Email string
	Papel papel.Papel
}

// Sessao valida o token e devolve quem ele identifica.
//
// A interface é declarada AQUI, no consumidor, e não importada do emissor: é o
// que evita o ciclo (os controllers de `iam/domain` importam este pacote) e o
// que permite testar o middleware com uma sessão falsa, sem assinar JWT.
//
// Erros esperados: ErrSessaoAusente, ErrSessaoInvalida, ErrSessaoExpirada —
// traduzidos pelo adaptador do `cmd/bootstrap`, que conhece os dois
// vocabulários.
type Sessao interface {
	Validar(bruto string) (Autor, error)
}

// Conferente confirma, no banco, que a conta do token ainda existe, está ativa
// e continua com o papel que o token afirma.
//
// Ele existe porque o token dura uma semana. Sem esta consulta, remover um
// usuário ou rebaixá-lo a visualizador só passaria a valer no próximo login
// dele — o que é inaceitável quando a razão de remover é justamente tirar o
// acesso agora. O preço é uma leitura por chave primária a cada requisição
// autenticada; num Postgres local, é menos do que o custo de montar o JSON da
// resposta.
type Conferente interface {
	// Situacao devolve o papel atual e se a conta está ativa.
	// ErrUsuarioDesconhecido quando a conta não existe mais.
	Situacao(ctx context.Context, id uuid.UUID) (papel.Papel, bool, error)
}

// Dependencias é o que o boot liga neste pacote.
type Dependencias struct {
	Sessao     Sessao
	Conferente Conferente
}
