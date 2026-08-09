// Package reqctx guarda no contexto do request o que atravessa todas as
// camadas sem ser argumento de função: hoje, o identificador de correlação
// (`ray_trace`).
//
// É o parente pobre do `tenantctx` do Atila, e de propósito: aqui não existe
// workspace, tenant nem usuário — um receptor de eventos de UM servidor de
// Valheim não tem escopo para carregar. O que sobra é a correlação: a mesma
// linha de log que entrou no `POST /eventos` precisa ser reconhecível no log de
// acesso, no evento de erro e na resposta ao cliente.
//
// Pacote folha: importa apenas a stdlib e o uuid.
package reqctx

import (
	"context"

	"github.com/google/uuid"
)

// chave é o tipo privado das chaves do contexto — impede colisão com chaves de
// outros pacotes (que não conseguem construir este tipo).
type chave int

const (
	chaveRayTrace chave = iota
	chaveIdentidade
)

// Identidade é quem está fazendo o request, resolvido pelo middleware a partir
// da sessão.
//
// Ela viaja no contexto, e não como parâmetro, porque atravessa camadas que não
// têm nada a ver com autenticação: o service que grava um usuário quer saber
// QUEM gravou, e não deveria receber isso por uma cadeia de argumentos que
// passa por todo mundo.
type Identidade struct {
	UsuarioUUID uuid.UUID
	Email       string
	Nome        string
	Papel       string
}

// Autenticado diz se há alguém identificado no contexto.
func (i Identidade) Autenticado() bool { return i.UsuarioUUID != uuid.Nil }

// ComIdentidade devolve um contexto derivado carregando quem é o autor do
// request.
func ComIdentidade(ctx context.Context, id Identidade) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, chaveIdentidade, id)
}

// Do devolve a identidade do contexto — zero quando não há sessão.
//
// Nunca devolve erro: o controle de acesso é do middleware, e um service que
// leia a identidade só para carimbar autoria não deve precisar tratar ausência.
func Do(ctx context.Context) Identidade {
	if ctx == nil {
		return Identidade{}
	}
	id, _ := ctx.Value(chaveIdentidade).(Identidade)
	return id
}

// ComRayTrace devolve um contexto derivado carregando o identificador de
// correlação. Valor vazio não entra: um ray_trace em branco no contexto é pior
// que a ausência, porque engana quem for lê-lo.
func ComRayTrace(ctx context.Context, rayTrace string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if rayTrace == "" {
		return ctx
	}
	return context.WithValue(ctx, chaveRayTrace, rayTrace)
}

// RayTrace lê o identificador de correlação do contexto ("" quando não houver).
func RayTrace(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	rayTrace, _ := ctx.Value(chaveRayTrace).(string)
	return rayTrace
}

// NovoRayTrace gera um identificador de correlação novo.
//
// UUID v4 em vez de ULID para não trazer dependência nova: o valor é opaco para
// quem o lê, e o que se faz com ele é procurar a mesma string em dois logs.
func NovoRayTrace() string { return uuid.NewString() }
