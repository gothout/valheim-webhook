// Package valheimsave lê os arquivos de save do Valheim: o mundo (`.db`) e o
// personagem (`.fch`).
//
// É folha: só a biblioteca padrão, nada de `internal/` fora de `pkg`.
//
// Nenhum parser público lê os formatos atuais — a implementação de referência
// em Java (Kakoen/valheim-save-tools) para na versão 34 do mundo e na 37 do
// personagem. O layout aqui foi conferido contra ela onde havia sobreposição e
// obtido por engenharia reversa de arquivos reais onde não havia; cada
// divergência está documentada no ponto em que aparece.
package valheimsave

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Vec3 é um Vector3 da Unity como está gravado no save (três float32).
type Vec3 struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

// Vec2s é a coordenada de zona/setor, gravada como dois int16.
type Vec2s struct {
	X, Y int16
}

// Quat é um Quaternion da Unity (quatro float32).
type Quat struct {
	X, Y, Z, W float32
}

// Reader decodifica o formato ZPackage do Valheim: primitivas little-endian e
// tamanho de string com prefixo codificado em 7 bits, no padrão .NET.
type Reader struct {
	buf []byte
	pos int
}

func NewReader(b []byte) *Reader { return &Reader{buf: b} }

func (r *Reader) Pos() int       { return r.pos }
func (r *Reader) Remaining() int { return len(r.buf) - r.pos }

// Seek move a posição de leitura, para quem precisa reinterpretar um trecho
// depois de descobrir que o layout não era o esperado.
func (r *Reader) Seek(pos int) error {
	if pos < 0 || pos > len(r.buf) {
		return fmt.Errorf("posicao %d fora do arquivo (%d bytes)", pos, len(r.buf))
	}
	r.pos = pos
	return nil
}

func (r *Reader) need(n int) error {
	if n < 0 || r.pos+n > len(r.buf) {
		return fmt.Errorf("fim inesperado dos dados no offset %d: precisa de %d bytes, restam %d", r.pos, n, len(r.buf)-r.pos)
	}
	return nil
}

func (r *Reader) Byte() (byte, error) {
	if err := r.need(1); err != nil {
		return 0, err
	}
	b := r.buf[r.pos]
	r.pos++
	return b, nil
}

func (r *Reader) Bool() (bool, error) {
	b, err := r.Byte()
	return b != 0, err
}

func (r *Reader) Int16() (int16, error) {
	if err := r.need(2); err != nil {
		return 0, err
	}
	v := int16(binary.LittleEndian.Uint16(r.buf[r.pos:]))
	r.pos += 2
	return v, nil
}

func (r *Reader) UInt16() (uint16, error) {
	if err := r.need(2); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint16(r.buf[r.pos:])
	r.pos += 2
	return v, nil
}

func (r *Reader) Int32() (int32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	v := int32(binary.LittleEndian.Uint32(r.buf[r.pos:]))
	r.pos += 4
	return v, nil
}

func (r *Reader) UInt32() (uint32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(r.buf[r.pos:])
	r.pos += 4
	return v, nil
}

func (r *Reader) Int64() (int64, error) {
	if err := r.need(8); err != nil {
		return 0, err
	}
	v := int64(binary.LittleEndian.Uint64(r.buf[r.pos:]))
	r.pos += 8
	return v, nil
}

func (r *Reader) Float32() (float32, error) {
	v, err := r.UInt32()
	return math.Float32frombits(v), err
}

func (r *Reader) Float64() (float64, error) {
	if err := r.need(8); err != nil {
		return 0, err
	}
	v := math.Float64frombits(binary.LittleEndian.Uint64(r.buf[r.pos:]))
	r.pos += 8
	return v, nil
}

func (r *Reader) Vec3() (Vec3, error) {
	var v Vec3
	var err error
	if v.X, err = r.Float32(); err != nil {
		return v, err
	}
	if v.Y, err = r.Float32(); err != nil {
		return v, err
	}
	v.Z, err = r.Float32()
	return v, err
}

func (r *Reader) Vec2s() (Vec2s, error) {
	var v Vec2s
	var err error
	if v.X, err = r.Int16(); err != nil {
		return v, err
	}
	v.Y, err = r.Int16()
	return v, err
}

func (r *Reader) Quat() (Quat, error) {
	var q Quat
	var err error
	if q.X, err = r.Float32(); err != nil {
		return q, err
	}
	if q.Y, err = r.Float32(); err != nil {
		return q, err
	}
	if q.Z, err = r.Float32(); err != nil {
		return q, err
	}
	q.W, err = r.Float32()
	return q, err
}

func (r *Reader) Bytes(n int) ([]byte, error) {
	if err := r.need(n); err != nil {
		return nil, err
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

// LengthPrefixedBytes lê um int32 de tamanho seguido dessa quantidade de bytes.
func (r *Reader) LengthPrefixedBytes() ([]byte, error) {
	n, err := r.Int32()
	if err != nil {
		return nil, err
	}
	return r.Bytes(int(n))
}

// stringLength decodifica o prefixo de tamanho em 7 bits do .NET.
func (r *Reader) stringLength() (int, error) {
	var value, shift int
	for i := 0; i < 5; i++ {
		b, err := r.Byte()
		if err != nil {
			return 0, err
		}
		value |= int(b&0x7F) << shift
		if b&0x80 == 0 {
			return value, nil
		}
		shift += 7
	}
	return 0, fmt.Errorf("prefixo de tamanho de string invalido no offset %d", r.pos)
}

func (r *Reader) Str() (string, error) {
	n, err := r.stringLength()
	if err != nil {
		return "", err
	}
	b, err := r.Bytes(n)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// NumItems lê o prefixo de contagem de propriedades. Mundo a partir da versão
// 33 usa uma forma compacta de um ou dois bytes; antes disso era um char UTF-8.
func (r *Reader) NumItems(worldVersion int32) (int, error) {
	if worldVersion < 33 {
		// Save anterior à 33 grava a contagem como um caractere UTF-8. Mundo
		// atual nunca cai aqui; falhar alto é melhor do que decodificar torto
		// em silêncio.
		return 0, fmt.Errorf("mundo versao %d nao suportado (anterior a 33)", worldVersion)
	}
	b, err := r.Byte()
	if err != nil {
		return 0, err
	}
	n := int(b)
	if n&128 != 0 {
		b2, err := r.Byte()
		if err != nil {
			return 0, err
		}
		n = ((n & 127) << 8) | int(b2)
	}
	return n, nil
}
