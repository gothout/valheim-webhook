package valheimsave

import (
	_ "embed"
	"strings"
	"sync"
	"unicode/utf16"
)

//go:embed known_strings.txt
var knownStrings string

// StableHash reproduz o StringExtensionMethods.GetStableHashCode do jogo.
//
// Nome de prefab e chave de propriedade são gravados como hash, não como
// texto: este é o único caminho de volta de um save para um nome legível.
//
// A iteração é sobre unidades UTF-16 porque é o que o `char` do C# significa —
// com nome ASCII dá no mesmo, com acento não.
func StableHash(s string) int32 {
	units := utf16.Encode([]rune(s))
	num1 := int32(5381)
	num2 := num1
	for i := 0; i < len(units); i += 2 {
		if units[i] == 0 {
			break
		}
		num1 = ((num1 << 5) + num1) ^ int32(units[i])
		if i == len(units)-1 || units[i+1] == 0 {
			break
		}
		num2 = ((num2 << 5) + num2) ^ int32(units[i+1])
	}
	return num1 + num2*1566083941
}

var (
	nameOnce   sync.Once
	nameByHash map[int32]string
)

func names() map[int32]string {
	nameOnce.Do(func() {
		nameByHash = make(map[int32]string, 2048)
		for _, line := range strings.Split(knownStrings, "\n") {
			name := strings.TrimSpace(line)
			if name == "" {
				continue
			}
			nameByHash[StableHash(name)] = name
		}
	})
	return nameByHash
}

// LookupName resolve um hash para o texto original, ou "" quando desconhecido.
//
// A tabela embutida cobre os prefabs e chaves comuns, não todo asset do jogo:
// resultado vazio é esperado e não é erro. Use RegisterNames para ampliá-la.
func LookupName(hash int32) string {
	return names()[hash]
}

// RegisterNames acrescenta textos à tabela reversa, para quem tiver uma lista
// de prefabs mais completa que a embutida.
func RegisterNames(extra []string) {
	m := names()
	for _, name := range extra {
		if name = strings.TrimSpace(name); name != "" {
			m[StableHash(name)] = name
		}
	}
}
