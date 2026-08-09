// Package web é o front-end do painel, EMBUTIDO no binário.
//
// A escolha de stack é uma escolha de operação: HTML servido pelo próprio Go
// (`html/template`) mais JavaScript de baunilha, sem passo de build, sem
// `node_modules` e sem CDN. O produto é um binário único que o dono do servidor
// copia para a máquina e roda — se o painel exigisse `npm run build` para
// existir, ele deixaria de ser um binário único.
//
// Sem CDN também é decisão: a máquina que roda um servidor de Valheim caseiro
// pode estar atrás de firewall, e um painel que só desenha quando alcança um
// CDN é um painel que falha exatamente quando se precisa dele.
//
// O pacote não importa nada de `internal/` — é uma folha, consumida por
// `cmd/server/routes`.
package web

import (
	"embed"
	"html/template"
	"io/fs"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Dados é o que as páginas recebem do servidor.
//
// É pouco de propósito: o resto o navegador busca pela API, com a sessão do
// usuário. O que vem daqui é só o que precisa estar na tela ANTES da primeira
// chamada — título e versão.
type Dados struct {
	Titulo   string
	Servidor string
	Versao   string
}

// Templates devolve as páginas já compiladas.
//
// Compilar UMA vez, no boot, e não a cada request: template é imutável depois
// de embutido, e recompilar por requisição seria trabalho puro. Erro aqui é
// fatal e aparece no boot — não em produção, na primeira visita.
func Templates() (*template.Template, error) {
	return template.ParseFS(templatesFS, "templates/*.html")
}

// Estaticos devolve o sistema de arquivos de `/static`, já com a raiz correta.
func Estaticos() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}
