// Package access_log é a trilha de acesso HTTP: uma linha por request, no log
// estruturado do processo.
//
// Ele é o PRIMEIRO middleware da cadeia porque é quem cria o `ray_trace` do
// request e o injeta no contexto — daí para frente, o observador de erros, a
// resposta de erro e o painel falam todos do mesmo identificador.
//
// Pacote folha: importa gin, reqctx (irmão) e stdlib. O destino é o `slog` do
// processo; um destino externo (banco de logs) entraria como interface
// declarada aqui, no consumidor, e ligada pelo boot — como no observador de
// erros. Enquanto não houver, `slog` basta e não custa dependência nenhuma.
package access_log

import (
	"log/slog"
	"time"
	"unicode/utf8"
)

// Entrada é a linha da trilha de acesso.
type Entrada struct {
	Ts        time.Time
	Metodo    string
	Rota      string
	Status    int
	DuracaoMs int64
	Bytes     int
	IP        string
	UserAgent string
	RayTrace  string
	ErroGin   string
}

// LimiteUserAgent corta o user-agent na gravação: é um campo que o cliente
// controla e que não deve poder inflar a linha de log.
const LimiteUserAgent = 200

// Registrar escreve a entrada no log do processo.
//
// O nível vem do status: 5xx é `error` (alguém precisa olhar), 4xx é `warn`
// (cliente errado — inclui o token de ingestão recusado) e o resto é `info`.
func Registrar(e Entrada) {
	msg := "[HTTP] " + e.Metodo + " " + e.Rota
	atributos := []any{
		"status", e.Status,
		"duracao_ms", e.DuracaoMs,
		"bytes", e.Bytes,
		"ip", e.IP,
		"user_agent", truncar(e.UserAgent, LimiteUserAgent),
		"ray_trace", e.RayTrace,
	}
	if e.ErroGin != "" {
		atributos = append(atributos, "erro", e.ErroGin)
	}

	switch {
	case e.Status >= 500:
		slog.Error(msg, atributos...)
	case e.Status >= 400:
		slog.Warn(msg, atributos...)
	default:
		slog.Info(msg, atributos...)
	}
}

// truncar corta o texto em `limite` bytes, sem partir rune ao meio.
func truncar(s string, limite int) string {
	if len(s) <= limite {
		return s
	}
	corte := limite
	for corte > 0 && !utf8.ValidString(s[:corte]) {
		corte--
	}
	return s[:corte] + "…"
}
