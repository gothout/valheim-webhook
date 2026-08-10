package valheimsave

import (
	"encoding/base64"
	"fmt"
)

// Item é uma pilha dentro de um container.
type Item struct {
	Name        string            `json:"name"`
	Stack       int32             `json:"stack"`
	Durability  float32           `json:"durability"`
	PosX        int32             `json:"posX"`
	PosY        int32             `json:"posY"`
	Equipped    bool              `json:"equipped"`
	Quality     int32             `json:"quality,omitempty"`
	Variant     int32             `json:"variant,omitempty"`
	CrafterID   int64             `json:"crafterId,omitempty"`
	CrafterName string            `json:"crafterName,omitempty"`
	CustomData  map[string]string `json:"customData,omitempty"`
	WorldLevel  int32             `json:"worldLevel,omitempty"`
	PickedUp    bool              `json:"pickedUp,omitempty"`
}

// Inventory é o conteúdo decodificado da propriedade `items` de um container.
type Inventory struct {
	Version int32  `json:"version"`
	Items   []Item `json:"items"`
}

// Inventory decodifica o conteúdo do container deste objeto.
//
// O Valheim grava o inventário numa propriedade de TEXTO chamada `items`, com
// um ZPackage aninhado em base64 — não num byteArray, que é onde a intuição
// manda procurar. O segundo retorno diz se o objeto é um container.
func (z *ZDO) Inventory() (*Inventory, bool, error) {
	if s, ok := z.String("items"); ok {
		raw, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, true, fmt.Errorf("decodificando base64 de items: %w", err)
		}
		inv, err := ParseInventory(raw)
		return inv, true, err
	}
	// Save antigo grava o bloco cru em vez de base64 num texto.
	if raw, ok := z.ByteArray("items"); ok {
		inv, err := ParseInventory(raw)
		return inv, true, err
	}
	return nil, false, nil
}

// ParseInventory decodifica o bloco de itens. O formato tem versionamento
// próprio, independente do save do mundo.
func ParseInventory(data []byte) (*Inventory, error) {
	r := NewReader(data)
	inv, err := parseInventoryFrom(r)
	if err != nil {
		return inv, err
	}
	if rem := r.Remaining(); rem != 0 {
		return inv, fmt.Errorf("inventario decodificado mas sobraram %d bytes (versao %d pode ter campos novos)", rem, inv.Version)
	}
	return inv, nil
}

// parseInventoryFrom decodifica um inventário na posição atual do leitor e o
// deixa logo depois dele. Usado quando o bloco está embutido numa estrutura
// maior, como o arquivo de personagem.
func parseInventoryFrom(r *Reader) (*Inventory, error) {
	version, err := r.Int32()
	if err != nil {
		return nil, fmt.Errorf("lendo versao do inventario: %w", err)
	}
	count, err := r.Int32()
	if err != nil {
		return nil, fmt.Errorf("lendo quantidade de itens: %w", err)
	}
	if count < 0 || count > 1<<16 {
		return nil, fmt.Errorf("quantidade de itens implausivel: %d", count)
	}

	inv := &Inventory{Version: version, Items: make([]Item, 0, count)}
	for i := int32(0); i < count; i++ {
		var it Item
		if it.Name, err = r.Str(); err != nil {
			return inv, fmt.Errorf("item %d nome: %w", i, err)
		}
		if it.Stack, err = r.Int32(); err != nil {
			return inv, fmt.Errorf("item %d (%s) stack: %w", i, it.Name, err)
		}
		if it.Durability, err = r.Float32(); err != nil {
			return inv, fmt.Errorf("item %d (%s) durabilidade: %w", i, it.Name, err)
		}
		if it.PosX, err = r.Int32(); err != nil {
			return inv, fmt.Errorf("item %d (%s) posX: %w", i, it.Name, err)
		}
		if it.PosY, err = r.Int32(); err != nil {
			return inv, fmt.Errorf("item %d (%s) posY: %w", i, it.Name, err)
		}
		if it.Equipped, err = r.Bool(); err != nil {
			return inv, fmt.Errorf("item %d (%s) equipped: %w", i, it.Name, err)
		}
		if version >= 101 {
			if it.Quality, err = r.Int32(); err != nil {
				return inv, fmt.Errorf("item %d (%s) quality: %w", i, it.Name, err)
			}
		}
		if version >= 102 {
			if it.Variant, err = r.Int32(); err != nil {
				return inv, fmt.Errorf("item %d (%s) variant: %w", i, it.Name, err)
			}
		}
		if version >= 103 {
			if it.CrafterID, err = r.Int64(); err != nil {
				return inv, fmt.Errorf("item %d (%s) crafterId: %w", i, it.Name, err)
			}
			if it.CrafterName, err = r.Str(); err != nil {
				return inv, fmt.Errorf("item %d (%s) crafterName: %w", i, it.Name, err)
			}
		}
		if version >= 104 {
			n, err := r.Int32()
			if err != nil {
				return inv, fmt.Errorf("item %d (%s) customData: %w", i, it.Name, err)
			}
			if n < 0 || n > 4096 {
				return inv, fmt.Errorf("item %d (%s) customData com tamanho implausivel: %d", i, it.Name, n)
			}
			if n > 0 {
				it.CustomData = make(map[string]string, n)
				for j := int32(0); j < n; j++ {
					k, err := r.Str()
					if err != nil {
						return inv, fmt.Errorf("item %d (%s) customData chave: %w", i, it.Name, err)
					}
					v, err := r.Str()
					if err != nil {
						return inv, fmt.Errorf("item %d (%s) customData valor: %w", i, it.Name, err)
					}
					it.CustomData[k] = v
				}
			}
		}
		if version >= 105 {
			if it.WorldLevel, err = r.Int32(); err != nil {
				return inv, fmt.Errorf("item %d (%s) worldLevel: %w", i, it.Name, err)
			}
		}
		if version >= 106 {
			if it.PickedUp, err = r.Bool(); err != nil {
				return inv, fmt.Errorf("item %d (%s) pickedUp: %w", i, it.Name, err)
			}
		}
		inv.Items = append(inv.Items, it)
	}
	return inv, nil
}
