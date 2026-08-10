package valheimsave

import (
	"fmt"
	"os"
)

// ZDO é um objeto em rede do mundo: um baú, uma parede, um portal, uma
// criatura. Tudo o que o servidor persiste é um destes.
type ZDO struct {
	Persistent bool
	Type       byte
	Distant    bool
	Prefab     int32
	Sector     Vec2s
	Position   Vec3

	HasRotation bool
	Rotation    Vec3

	Floats     map[int32]float32
	Vec3s      map[int32]Vec3
	Quats      map[int32]Quat
	Ints       map[int32]int32
	Longs      map[int32]int64
	Strings    map[int32]string
	ByteArrays map[int32][]byte

	SaveConnections bool
	ConnectionType  byte
	ConnectionHash  int32
}

// Name resolve o hash do prefab para um nome legível, ou "" quando ele não
// está na tabela embutida.
func (z *ZDO) Name() string { return LookupName(z.Prefab) }

// Int devolve uma propriedade inteira pelo nome da chave.
func (z *ZDO) Int(key string) (int32, bool) {
	v, ok := z.Ints[StableHash(key)]
	return v, ok
}

// Float devolve uma propriedade float pelo nome da chave.
func (z *ZDO) Float(key string) (float32, bool) {
	v, ok := z.Floats[StableHash(key)]
	return v, ok
}

// String devolve uma propriedade de texto pelo nome da chave.
func (z *ZDO) String(key string) (string, bool) {
	v, ok := z.Strings[StableHash(key)]
	return v, ok
}

// ByteArray devolve uma propriedade crua pelo nome da chave.
func (z *ZDO) ByteArray(key string) ([]byte, bool) {
	v, ok := z.ByteArrays[StableHash(key)]
	return v, ok
}

// World é o conteúdo decodificado de um save `<mundo>.db`.
type World struct {
	Version int32
	NetTime float64
	MyID    int64
	NextUID uint32
	ZDOs    []ZDO

	// TrailingBytes é o que vem depois da tabela de ZDOs (estado de zonas e
	// de eventos aleatórios). Fica preservado mas não é decodificado, porque
	// nada aqui precisa dele.
	TrailingBytes int
}

// MinWorldVersion é o layout mais antigo aceito. A contagem compacta de
// propriedades entrou na 33; save anterior precisa de outro caminho de
// leitura, que não está implementado.
const MinWorldVersion = 33

// ParseWorldFile lê e decodifica um save de mundo `.db`.
func ParseWorldFile(path string) (*World, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseWorld(data)
}

// ParseWorld decodifica um save de mundo `.db` já em memória.
func ParseWorld(data []byte) (*World, error) {
	r := NewReader(data)
	w := &World{}

	var err error
	if w.Version, err = r.Int32(); err != nil {
		return nil, fmt.Errorf("lendo versao do mundo: %w", err)
	}
	if w.Version < MinWorldVersion {
		return nil, fmt.Errorf("versao de mundo %d nao suportada (minimo %d)", w.Version, MinWorldVersion)
	}
	if w.NetTime, err = r.Float64(); err != nil {
		return nil, fmt.Errorf("lendo netTime: %w", err)
	}
	if w.MyID, err = r.Int64(); err != nil {
		return nil, fmt.Errorf("lendo myId: %w", err)
	}
	if w.NextUID, err = r.UInt32(); err != nil {
		return nil, fmt.Errorf("lendo nextUid: %w", err)
	}

	count, err := r.Int32()
	if err != nil {
		return nil, fmt.Errorf("lendo quantidade de zdos: %w", err)
	}
	if count < 0 {
		return nil, fmt.Errorf("quantidade de zdos invalida: %d", count)
	}

	w.ZDOs = make([]ZDO, 0, count)
	for i := int32(0); i < count; i++ {
		var z ZDO
		if err := readZDO(r, w.Version, &z); err != nil {
			return nil, fmt.Errorf("zdo %d de %d (offset %d): %w", i, count, r.Pos(), err)
		}
		w.ZDOs = append(w.ZDOs, z)
	}

	w.TrailingBytes = r.Remaining()
	return w, nil
}

const (
	flagConnections = 1 << 0
	flagFloats      = 1 << 1
	flagVec3s       = 1 << 2
	flagQuats       = 1 << 3
	flagInts        = 1 << 4
	flagLongs       = 1 << 5
	flagStrings     = 1 << 6
	flagByteArrays  = 1 << 7
	flagPersistent  = 1 << 8
	flagDistant     = 1 << 9
	flagRotation    = 1 << 12
)

func readZDO(r *Reader, ver int32, z *ZDO) error {
	flags, err := r.UInt16()
	if err != nil {
		return err
	}
	z.Persistent = flags&flagPersistent != 0
	z.Distant = flags&flagDistant != 0
	z.Type = byte((flags >> 10) & 3)

	if z.Sector, err = r.Vec2s(); err != nil {
		return err
	}
	if z.Position, err = r.Vec3(); err != nil {
		return err
	}
	if z.Prefab, err = r.Int32(); err != nil {
		return err
	}

	z.HasRotation = flags&flagRotation != 0
	if z.HasRotation {
		if z.Rotation, err = r.Vec3(); err != nil {
			return err
		}
	}

	// O byte baixo concentra todas as flags de "tem dado". Com ele zerado o
	// ZDO não carrega propriedade nenhuma e termina aqui.
	if flags&0xFF == 0 {
		return nil
	}

	z.SaveConnections = flags&flagConnections != 0
	if z.SaveConnections {
		if z.ConnectionType, err = r.Byte(); err != nil {
			return err
		}
		if z.ConnectionHash, err = r.Int32(); err != nil {
			return err
		}
	}

	if flags&flagFloats != 0 {
		if z.Floats, err = readProps(r, ver, (*Reader).Float32); err != nil {
			return fmt.Errorf("floats: %w", err)
		}
	}
	if flags&flagVec3s != 0 {
		if z.Vec3s, err = readProps(r, ver, (*Reader).Vec3); err != nil {
			return fmt.Errorf("vector3s: %w", err)
		}
	}
	if flags&flagQuats != 0 {
		if z.Quats, err = readProps(r, ver, (*Reader).Quat); err != nil {
			return fmt.Errorf("quaternions: %w", err)
		}
	}
	if flags&flagInts != 0 {
		if z.Ints, err = readProps(r, ver, (*Reader).Int32); err != nil {
			return fmt.Errorf("ints: %w", err)
		}
	}
	if flags&flagLongs != 0 {
		if z.Longs, err = readProps(r, ver, (*Reader).Int64); err != nil {
			return fmt.Errorf("longs: %w", err)
		}
	}
	if flags&flagStrings != 0 {
		if z.Strings, err = readProps(r, ver, (*Reader).Str); err != nil {
			return fmt.Errorf("strings: %w", err)
		}
	}
	if flags&flagByteArrays != 0 {
		if z.ByteArrays, err = readProps(r, ver, (*Reader).LengthPrefixedBytes); err != nil {
			return fmt.Errorf("byteArrays: %w", err)
		}
	}
	return nil
}

// readProps lê uma tabela de propriedades indexada por hash: uma contagem e,
// em seguida, essa quantidade de pares (chave int32, valor).
func readProps[T any](r *Reader, ver int32, read func(*Reader) (T, error)) (map[int32]T, error) {
	n, err := r.NumItems(ver)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	m := make(map[int32]T, n)
	for i := 0; i < n; i++ {
		key, err := r.Int32()
		if err != nil {
			return nil, err
		}
		v, err := read(r)
		if err != nil {
			return nil, err
		}
		m[key] = v
	}
	return m, nil
}
