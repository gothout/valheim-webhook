package routes

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/pkg/config"
	"valheim-webhook/web"
)

// Rotas do painel.
const (
	RotaPainel      = "/"
	RotaLogin       = "/login"
	RotaAdmin       = "/admin"
	PrefixoEstatico = "/static"
)

// RegistrarPainel serve o front-end embutido no binário.
//
// As três páginas são HTML estático com o título preenchido no servidor; a
// autorização é feita pela API que elas chamam. É por isso que `/admin` não
// exige sessão AQUI: a página é uma casca, e cada chamada que ela faz é barrada
// (403) se quem abriu não for administrador. Proteger a casca também seria
// defensável, mas não é o que protege — proteger o dado é.
func RegistrarPainel(engine *gin.Engine, cfg *config.Config) error {
	if !cfg.Painel.Enabled {
		slog.Info("[PAINEL] desligado na configuração (painel.enabled=false): só a API responde")
		return nil
	}

	paginas, err := web.Templates()
	if err != nil {
		return fmt.Errorf("compilar as páginas do painel: %w", err)
	}
	estaticos, err := web.Estaticos()
	if err != nil {
		return fmt.Errorf("abrir os arquivos do painel: %w", err)
	}

	dados := web.Dados{
		Titulo:   cfg.Painel.Titulo,
		Servidor: cfg.Ingestao.Servidor,
		Versao:   cfg.App.Version,
	}

	engine.GET(RotaPainel, pagina(paginas, "painel.html", dados))
	engine.GET(RotaLogin, pagina(paginas, "login.html", dados))
	engine.GET(RotaAdmin, pagina(paginas, "admin.html", dados))
	engine.StaticFS(PrefixoEstatico, http.FS(estaticos))

	return nil
}

// pagina devolve o handler de uma página do painel.
//
// O HTML é renderizado em MEMÓRIA antes de ir para a rede: um erro de template
// no meio da escrita deixaria a resposta pela metade, com 200 já enviado e a
// página quebrada no navegador. Renderizando antes, o erro vira 500 inteiro.
func pagina(paginas *template.Template, nome string, dados web.Dados) gin.HandlerFunc {
	return func(c *gin.Context) {
		var corpo bytes.Buffer
		if err := paginas.ExecuteTemplate(&corpo, nome, dados); err != nil {
			slog.Error("[PAINEL] falha ao renderizar a página", "pagina", nome, "erro", err)
			c.String(http.StatusInternalServerError, "erro ao montar o painel")
			return
		}

		// Sem cache: o HTML é pequeno e muda quando o binário muda. O que
		// realmente pesa (CSS e JS) é servido pelo StaticFS, que já responde
		// 304 pela data de modificação.
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", corpo.Bytes())
	}
}
