package migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// SufixoUp e SufixoDown são as duas metades de toda migration.
const (
	SufixoUp   = ".up.sql"
	SufixoDown = ".down.sql"
)

// Arquivo é uma migration do diretório, já reconhecida.
type Arquivo struct {
	Versao uint
	Nome   string
	// TemDown é falso quando o par `.down.sql` não existe — o que reprova a
	// validação: migration sem volta é decisão que só se descobre no pior dia.
	TemDown bool
}

// Listar lê o diretório de migrations e devolve os arquivos em ordem de versão.
//
// Reconhece o padrão `NNNN_descricao.{up,down}.sql`. Arquivo com nome fora do
// padrão vira erro em vez de ser ignorado: um `0007_algo.sql` (sem o `up`)
// nunca seria aplicado, e descobrir isso em produção é caro.
func Listar(caminho string) ([]Arquivo, error) {
	entradas, err := os.ReadDir(caminho)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrDiretorio, caminho, err)
	}

	porVersao := make(map[uint]*Arquivo)
	for _, entrada := range entradas {
		if entrada.IsDir() {
			continue
		}
		nome := entrada.Name()
		if !strings.HasSuffix(nome, ".sql") {
			continue
		}

		subida := strings.HasSuffix(nome, SufixoUp)
		descida := strings.HasSuffix(nome, SufixoDown)
		if !subida && !descida {
			return nil, fmt.Errorf("%w: %s (esperado %s ou %s)",
				ErrNomeInvalido, nome, SufixoUp, SufixoDown)
		}

		base := strings.TrimSuffix(strings.TrimSuffix(nome, SufixoUp), SufixoDown)
		partes := strings.SplitN(base, "_", 2)
		if len(partes) != 2 {
			return nil, fmt.Errorf("%w: %s (esperado NNNN_descricao%s)", ErrNomeInvalido, nome, SufixoUp)
		}
		numero, err := strconv.ParseUint(partes[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %s (versão %q não é um número)", ErrNomeInvalido, nome, partes[0])
		}

		versao := uint(numero)
		arquivo := porVersao[versao]
		if arquivo == nil {
			arquivo = &Arquivo{Versao: versao, Nome: partes[1]}
			porVersao[versao] = arquivo
		}
		if descida {
			arquivo.TemDown = true
		}
	}

	lista := make([]Arquivo, 0, len(porVersao))
	for _, arquivo := range porVersao {
		lista = append(lista, *arquivo)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Versao < lista[j].Versao })
	return lista, nil
}

// Validar confere o diretório sem abrir conexão nenhuma: roda com o banco fora
// do ar, e é o que entra no `go test` e no CI.
//
// Cobra três coisas: versões únicas, sequência sem buraco e `down` para todo
// `up`.
func Validar(caminho string) ([]Arquivo, error) {
	absoluto, err := filepath.Abs(strings.TrimSpace(caminho))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrDiretorio, caminho, err)
	}

	arquivos, err := Listar(absoluto)
	if err != nil {
		return nil, err
	}
	if len(arquivos) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrSemMigrations, absoluto)
	}

	esperada := uint(1)
	for _, arquivo := range arquivos {
		if arquivo.Versao != esperada {
			return nil, fmt.Errorf("%w: esperava a versão %04d e encontrei %04d (%s)",
				ErrSequencia, esperada, arquivo.Versao, arquivo.Nome)
		}
		if !arquivo.TemDown {
			return nil, fmt.Errorf("%w: %04d_%s%s não tem o par %s",
				ErrSemDown, arquivo.Versao, arquivo.Nome, SufixoUp, SufixoDown)
		}
		esperada++
	}
	return arquivos, nil
}
