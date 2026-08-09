package routes

import (
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"valheim-webhook/internal/pkg/config"
)

// RotaStatus é o health check. Fica fora dos grupos e não exige sessão: quem a
// chama é o `healthcheck` do container, que não faz login.
const RotaStatus = "/api/status"

// Estados possíveis de uma dependência no health check.
const (
	EstadoOK        = "ok"
	EstadoDegradado = "degradado"
)

// Sonda informa o estado de uma dependência.
//
// É uma função injetada pelo boot porque este pacote não importa `infra`: quem
// conhece as conexões é o `cmd/bootstrap`. A sonda roda a cada chamada do
// health check — não pode fazer I/O de rede.
type Sonda func() string

// StatusResponse é o corpo do GET /api/status.
type StatusResponse struct {
	Status       string            `json:"status" example:"ok"`
	App          string            `json:"app" example:"valheim-webhook"`
	Version      string            `json:"version" example:"0.1.0"`
	Env          string            `json:"env" example:"dev"`
	UptimeSec    int64             `json:"uptime_sec" example:"3600"`
	Time         time.Time         `json:"time"`
	Dependencias map[string]string `json:"dependencias,omitempty"`
}

// statusHandler responde o health check.
//
// Responde 200 mesmo com dependência degradada: o processo está vivo e serve o
// que consegue. Health check que devolve 503 porque o Discord está fora faria o
// Docker reiniciar um processo saudável — e reiniciar não conserta o Discord.
//
// @Summary  Health check
// @Tags     Plataforma · Status
// @Produce  json
// @Success  200 {object} routes.StatusResponse
// @Router   /api/status [get]
func statusHandler(cfg *config.Config, sondas map[string]Sonda, inicio time.Time) gin.HandlerFunc {
	// Ordem estável das chaves: o mapa da resposta é montado a cada request,
	// mas a lista de sondas é fixada aqui, no registro da rota.
	nomes := make([]string, 0, len(sondas))
	for nome := range sondas {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)

	return func(c *gin.Context) {
		agora := time.Now().UTC()

		var dependencias map[string]string
		if len(nomes) > 0 {
			dependencias = make(map[string]string, len(nomes))
			for _, nome := range nomes {
				dependencias[nome] = sondas[nome]()
			}
		}

		c.JSON(http.StatusOK, StatusResponse{
			Status:       EstadoOK,
			App:          cfg.App.Name,
			Version:      cfg.App.Version,
			Env:          cfg.App.Env,
			UptimeSec:    int64(agora.Sub(inicio).Seconds()),
			Time:         agora,
			Dependencias: dependencias,
		})
	}
}
