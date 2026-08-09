package memoria

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// Handler é um `slog.Handler` que ESPELHA no anel tudo o que passa por ele e
// repassa para o handler de baixo.
//
// Espelhar, e não substituir, é a decisão importante: o stdout continua sendo o
// log de verdade (é o que o `docker logs` recolhe e o que sobrevive ao processo
// morrer), e o anel é uma cópia para a tela.
type Handler struct {
	base   slog.Handler
	buffer *Buffer
	// attrs são os atributos herdados de um `With(...)`.
	attrs []slog.Attr
	// grupo é o prefixo dos nomes, acumulado pelos `WithGroup(...)`.
	grupo string
}

// NovoHandler embrulha o handler de baixo.
func NovoHandler(base slog.Handler, buffer *Buffer) *Handler {
	return &Handler{base: base, buffer: buffer}
}

// Enabled delega: o anel não decide o que é logado, só guarda o que foi.
func (h *Handler) Enabled(ctx context.Context, nivel slog.Level) bool {
	return h.base.Enabled(ctx, nivel)
}

// Handle grava no anel e repassa.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	linha := Linha{
		Ts:        r.Time.UTC(),
		Nivel:     r.Level.String(),
		Mensagem:  r.Message,
		Atributos: make(map[string]string, r.NumAttrs()+len(h.attrs)),
	}
	for _, a := range h.attrs {
		anexarAtributo(linha.Atributos, h.grupo, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		anexarAtributo(linha.Atributos, h.grupo, a)
		return true
	})
	if len(linha.Atributos) == 0 {
		linha.Atributos = nil
	}

	h.buffer.Anexar(linha)
	return h.base.Handle(ctx, r)
}

// WithAttrs devolve um handler com os atributos herdados.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	novo := *h
	novo.base = h.base.WithAttrs(attrs)
	novo.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &novo
}

// WithGroup devolve um handler com o prefixo de grupo.
func (h *Handler) WithGroup(nome string) slog.Handler {
	if nome == "" {
		return h
	}
	novo := *h
	novo.base = h.base.WithGroup(nome)
	novo.grupo = juntar(h.grupo, nome)
	return &novo
}

// anexarAtributo achata o atributo em texto, abrindo os grupos.
func anexarAtributo(destino map[string]string, prefixo string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		for _, filho := range a.Value.Group() {
			anexarAtributo(destino, juntar(prefixo, a.Key), filho)
		}
		return
	}
	destino[juntar(prefixo, a.Key)] = fmt.Sprint(a.Value.Any())
}

// juntar monta `grupo.chave`, sem ponto solto nas pontas.
func juntar(prefixo, nome string) string {
	switch {
	case prefixo == "":
		return nome
	case nome == "":
		return prefixo
	default:
		return strings.Join([]string{prefixo, nome}, ".")
	}
}
