package routes

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/pkg/config"
)

// CabecalhosAceitos são os headers que um cliente pode mandar.
var CabecalhosAceitos = []string{
	"Content-Type", "Authorization", "X-Request-Id", "X-Ingest-Token",
}

// CORS decide quais origens podem chamar a API de dentro de um navegador.
//
// Três formas de liberar, da mais restrita para a mais larga:
//
//  1. `painel.dominio` — o endereço público do painel entra sozinho, sem
//     ninguém precisar repeti-lo na lista de origens;
//  2. `app.base_domain` — qualquer origem sob `.atila.cloud` é aceita, então um
//     subdomínio novo não exige mexer na configuração. É o mesmo desenho do
//     Atila, e o ponto na frente do sufixo é o que impede `maliciosoatila.cloud`
//     de passar;
//  3. `server.http.cors.allowed_origins` — a lista exata, para o que não cabe
//     nas duas anteriores.
//
// Nada configurado = nenhuma origem externa, que é o padrão certo: o painel é
// servido pelo mesmo processo, na mesma origem, e não precisa de CORS nenhum.
//
// A implementação é de trinta linhas em vez de uma dependência — o que este
// produto precisa de CORS é exatamente isto.
func CORS(cfg *config.Config) gin.HandlerFunc {
	permitidas := make(map[string]struct{}, len(cfg.Server.HTTP.CORS.AllowedOrigins))
	curinga := false
	for _, origem := range cfg.Server.HTTP.CORS.AllowedOrigins {
		limpa := strings.ToLower(strings.TrimSpace(origem))
		if limpa == "*" {
			curinga = true
			continue
		}
		if limpa != "" {
			permitidas[limpa] = struct{}{}
		}
	}

	sufixo := cfg.App.SufixoDeOrigem()
	painel := strings.ToLower(strings.TrimSpace(cfg.Painel.Dominio))

	return func(c *gin.Context) {
		origem := c.GetHeader("Origin")
		if origem == "" {
			c.Next() // requisição de mesma origem: não é caso de CORS
			return
		}

		if !aceita(origem, permitidas, curinga, sufixo, painel) {
			// Não abortamos: sem os cabeçalhos de CORS, o NAVEGADOR já recusa a
			// leitura da resposta. Abortar aqui quebraria clientes que não são
			// navegador (e que, por isso, nem mandariam `Origin`).
			c.Next()
			return
		}

		// Eco da origem, nunca `*`: com credenciais (o cookie de sessão), o
		// curinga é recusado pelo próprio navegador.
		c.Header("Access-Control-Allow-Origin", origem)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", strings.Join(CabecalhosAceitos, ", "))
		c.Header("Vary", "Origin")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// aceita aplica as três formas de liberação.
func aceita(origem string, permitidas map[string]struct{}, curinga bool, sufixo, painel string) bool {
	if curinga {
		return true
	}
	if _, listada := permitidas[strings.ToLower(strings.TrimSpace(origem))]; listada {
		return true
	}

	// A comparação é sobre o HOST, não sobre a string inteira: `Origin` traz
	// esquema e porta, e compará-la crua obrigaria a listar `https://x` e
	// `http://x` como se fossem coisas diferentes de configurar.
	endereco, err := url.Parse(origem)
	if err != nil {
		return false
	}
	host := strings.ToLower(endereco.Hostname())
	if host == "" {
		return false
	}

	if painel != "" && host == painel {
		return true
	}
	return sufixo != "" && strings.HasSuffix(host, sufixo)
}

// avisarCORSAberto registra no boot que a configuração liberou qualquer origem.
func avisarCORSAberto(cfg *config.Config) {
	for _, origem := range cfg.Server.HTTP.CORS.AllowedOrigins {
		if strings.TrimSpace(origem) == "*" {
			slog.Warn("[CORS] qualquer origem está liberada (allowed_origins contém \"*\"): " +
				"qualquer site aberto no navegador de um usuário logado consegue chamar esta API")
			return
		}
	}
	if sufixo := cfg.App.SufixoDeOrigem(); sufixo != "" {
		slog.Info("[CORS] origens aceitas por sufixo do domínio-base", "sufixo", sufixo)
	}
}
