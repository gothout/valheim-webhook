package migrations

import "errors"

var (
	// ErrConexaoNula é o uso incorreto do runner (bug de quem chama).
	ErrConexaoNula = errors.New("conexão de migração nula")
	// ErrDiretorio indica diretório de migrations inacessível.
	ErrDiretorio = errors.New("diretório de migrations inválido")
	// ErrSemMigrations indica diretório vazio.
	ErrSemMigrations = errors.New("nenhuma migration encontrada")
	// ErrNomeInvalido indica arquivo fora do padrão NNNN_descricao.{up,down}.sql.
	ErrNomeInvalido = errors.New("nome de migration inválido")
	// ErrSequencia indica buraco na numeração.
	ErrSequencia = errors.New("sequência de migrations com buraco")
	// ErrSemDown indica `up` sem o `down` correspondente.
	ErrSemDown = errors.New("migration sem rollback")
	// ErrSchemaSujo indica que uma migration falhou no meio e o schema não é
	// confiável — o conserto é manual (`migrate force VERSAO`).
	ErrSchemaSujo = errors.New("schema sujo: uma migration falhou no meio")
)
