#!/usr/bin/env bash
#
# iniciar.sh — sobe o valheim-webhook (serviço + Postgres) em container.
#
# Na primeira execução ele cria o .env com os dois segredos sorteados; nas
# seguintes, respeita o que já está lá. É idempotente: rodar de novo atualiza a
# imagem e reinicia, sem tocar nos dados nem nos segredos.
#
#   ./iniciar.sh              sobe (ou atualiza) tudo
#   ./iniciar.sh --recriar    força rebuild da imagem
#
set -euo pipefail

cd "$(dirname "$0")"

ENV_FILE=".env"
COMPOSE=(docker compose)

azul()     { printf '\033[36m%s\033[0m\n' "$*"; }
verde()    { printf '\033[32m%s\033[0m\n' "$*"; }
amarelo()  { printf '\033[33m%s\033[0m\n' "$*"; }
vermelho() { printf '\033[31m%s\033[0m\n' "$*" >&2; }

# ---------- pré-requisitos ----------

if ! command -v docker >/dev/null 2>&1; then
  vermelho "docker não encontrado. Instale o Docker antes de continuar."
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  # Compose v1 (`docker-compose`) ainda existe em máquinas antigas.
  if command -v docker-compose >/dev/null 2>&1; then
    COMPOSE=(docker-compose)
  else
    vermelho "docker compose não encontrado (nem o plugin v2, nem o docker-compose v1)."
    exit 1
  fi
fi

# ---------- segredos ----------

# Sorteia um segredo em base64 seguro para URL. openssl está em qualquer
# máquina com Docker; o /dev/urandom é a rede de segurança.
sortear() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32 | tr -d '\n=+/' | cut -c1-40
  else
    head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
  fi
}

if [[ ! -f "$ENV_FILE" ]]; then
  azul "==> primeira execução: criando $ENV_FILE com segredos novos"
  cp .env.example "$ENV_FILE"

  # `|` como separador do sed: o segredo pode conter `/`.
  sed -i.bak "s|^JWT_SECRET=.*|JWT_SECRET=$(sortear)|"     "$ENV_FILE"
  sed -i.bak "s|^INGEST_TOKEN=.*|INGEST_TOKEN=$(sortear)|" "$ENV_FILE"
  rm -f "$ENV_FILE.bak"

  chmod 600 "$ENV_FILE"
  verde "    $ENV_FILE criado. Ele guarda os segredos: não versione."
fi

# shellcheck disable=SC1090
set -a; source "$ENV_FILE"; set +a

# ---------- subir ----------

if [[ "${1:-}" == "--recriar" ]]; then
  azul "==> rebuild da imagem"
  "${COMPOSE[@]}" build --no-cache
fi

azul "==> subindo os containers"
"${COMPOSE[@]}" up -d --build

# ---------- esperar ficar de pé ----------

PORTA="${HTTP_PORT:-6060}"
azul "==> aguardando o serviço responder em http://localhost:${PORTA}/api/status"

pronto=0
for _ in $(seq 1 60); do
  if curl -sf "http://localhost:${PORTA}/api/status" >/dev/null 2>&1; then
    pronto=1
    break
  fi
  sleep 1
done

if [[ "$pronto" -ne 1 ]]; then
  vermelho "o serviço não respondeu a tempo. Veja o que aconteceu:"
  vermelho "  ${COMPOSE[*]} logs app"
  exit 1
fi

verde "==> no ar"

# ---------- o que a pessoa precisa saber agora ----------

echo
echo "  painel .......... http://localhost:${PORTA}/"
echo "  ingestão ........ POST http://localhost:${PORTA}/eventos"
echo "  status .......... http://localhost:${PORTA}/api/status"
echo

# A senha do administrador aparece UMA vez no log, no primeiro boot. Pescá-la
# aqui evita que ela se perca no meio das outras linhas.
if "${COMPOSE[@]}" logs app 2>/dev/null | grep -q "CONTA ADMINISTRATIVA"; then
  amarelo "  conta administrativa criada neste boot — anote a senha:"
  "${COMPOSE[@]}" logs app 2>/dev/null | grep -A 6 "CONTA ADMINISTRATIVA" | sed 's/^/    /'
  echo
fi

echo "  Para o servidor de Valheim entregar os eventos, suba-o assim"
echo "  (o token é o INGEST_TOKEN do seu .env):"
echo
cat <<EOF
    docker run -d \\
      --name valheim-server \\
      --restart unless-stopped \\
      --cap-add=sys_nice \\
      --stop-timeout 120 \\
      -p 2456-2457:2456-2457/udp \\
      --add-host host.docker.internal:host-gateway \\
      -v /opt/valheim/config:/config \\
      -v /opt/valheim/worlds:/config/worlds_local \\
      -v /opt/valheim/data:/opt/valheim \\
      -e SERVER_NAME="${SERVIDOR:-playground}" \\
      -e WORLD_NAME="${SERVIDOR:-playground}" \\
      -e SERVER_PASS="troque_esta_senha" \\
      -e CROSSPLAY=true \\
      -e VALHEIM_LOG_FILTER_REGEXP_Eventos='Got character ZDOID from|Destroying abandoned non.persistent zdo|Got text|Game server connected' \\
      -e ON_VALHEIM_LOG_FILTER_REGEXP_Eventos='curl -sfSL -X POST -H "Content-Type: text/plain" -H "X-Ingest-Token: ${INGEST_TOKEN}" --data-binary @- "http://host.docker.internal:${PORTA}/eventos"' \\
      ghcr.io/community-valheim-tools/valheim-server
EOF
echo
echo "  (o README explica por que REGEXP e text/plain, e não MATCHES e JSON)"
