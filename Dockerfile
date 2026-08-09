# Imagem do valheim-webhook: build em duas etapas, imagem final mínima.
#
# O binário embute o painel inteiro (HTML, CSS e JS, via `go:embed`), então a
# imagem final não precisa de nada além do executável, dos certificados (para
# falar HTTPS com o Discord), do fuso horário e das migrations.

# ---------- etapa 1: build ----------
#
# O toolchain é 1.25 mesmo com `go 1.24.5` no go.mod (compilador novo constrói
# linguagem antiga sem problema). O motivo é concreto: uma dependência de TESTE
# transitiva (`rogpeppe/go-internal`, puxada pela cadeia do testify) exige
# `go >= 1.25`, e a imagem alpine fixa `GOTOOLCHAIN=local` — com 1.24 a
# resolução de módulos falha antes de compilar uma linha.
FROM golang:1.25-alpine AS build

WORKDIR /src

# As dependências vêm ANTES do código: enquanto go.mod/go.sum não mudarem, esta
# camada é reaproveitada e um rebuild não volta à rede.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO desligado: o binário fica estático e roda em qualquer imagem base.
# -trimpath tira o caminho da máquina de build; -ldflags="-s -w" tira a tabela
# de símbolos, que não serve para nada em produção e pesa alguns MB.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/valheim-webhook .

# ---------- etapa 2: imagem final ----------
FROM alpine:3.20

# ca-certificates: sem eles, a chamada HTTPS ao Discord falha com erro de
# certificado — e o sintoma (mensagem nunca chega) não aponta para a causa.
# tzdata: o carimbo das linhas de log do Valheim vem na hora LOCAL, e é o TZ do
# container que diz qual é ela.
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S valheim && adduser -S -G valheim valheim

WORKDIR /app

COPY --from=build /out/valheim-webhook /usr/local/bin/valheim-webhook
COPY db/migrations /app/db/migrations
COPY configs.docker.json /etc/valheim-webhook/configs.json

USER valheim

EXPOSE 6060

# O health check é o mesmo endereço que o orquestrador usaria: ele responde 200
# enquanto o processo estiver vivo, com o estado das dependências no corpo.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:6060/api/status >/dev/null || exit 1

ENTRYPOINT ["valheim-webhook"]
CMD ["serve"]
