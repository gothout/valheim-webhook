# valheim-webhook

Recebe as linhas de log de um servidor de Valheim
([valheim-server-docker](https://github.com/community-valheim-tools/valheim-server-docker)),
entende o que aconteceu, guarda, mostra num painel web e avisa no Discord.

```
┌──────────────────────┐   POST /eventos    ┌──────────────────┐   embed    ┌─────────┐
│ valheim-server (hook)│ ─────────────────▶ │ valheim-webhook  │ ─────────▶ │ Discord │
└──────────────────────┘   (linha de log)   │  Go + Postgres   │            └─────────┘
                                            │  painel embutido │ ── SSE ──▶  navegador
                                            └──────────────────┘
```

O que ele reconhece hoje: **entrou**, **saiu**, **morreu**, **mensagem no chat**,
**conexão** e **servidor no ar**. Linha que ele não entende é guardada inteira,
como `desconhecido` — nunca descartada.

---

## Subir

Requisitos: Docker e Docker Compose. Nada de Go, Node ou Postgres na máquina —
o serviço e o banco sobem em container.

```bash
./iniciar.sh
```

Na primeira execução ele cria o `.env` com os segredos sorteados, sobe tudo,
espera ficar de pé e imprime:

- o endereço do painel (`http://localhost:6060/`);
- **a senha do administrador**, que aparece uma única vez;
- o `docker run` pronto para o servidor de Valheim, já com o seu token.

Depois disso: `docker compose logs -f app` para acompanhar, `docker compose down`
para parar, `./iniciar.sh` de novo para atualizar.

---

## Ligar o servidor de Valheim nele

```bash
docker run -d \
  --name valheim-server \
  --restart unless-stopped \
  --cap-add=sys_nice \
  --stop-timeout 120 \
  -p 2456-2457:2456-2457/udp \
  --add-host host.docker.internal:host-gateway \
  -v /opt/valheim/config:/config \
  -v /opt/valheim/worlds:/config/worlds_local \
  -v /opt/valheim/data:/opt/valheim \
  -e SERVER_NAME="playground" \
  -e WORLD_NAME="playground" \
  -e SERVER_PASS="playground4_" \
  -e CROSSPLAY=true \
  -e VALHEIM_LOG_FILTER_REGEXP_Eventos='Got character ZDOID from|Destroying abandoned non.persistent zdo|Got text|Game server connected' \
  -e ON_VALHEIM_LOG_FILTER_REGEXP_Eventos='curl -sfSL -X POST -H "Content-Type: text/plain" -H "X-Ingest-Token: SEU_TOKEN" --data-binary @- "http://host.docker.internal:6060/eventos"' \
  ghcr.io/community-valheim-tools/valheim-server
```

### Duas mudanças em relação ao comando que você usou

O comando original era:

```bash
-e VALHEIM_LOG_FILTER_MATCHES_Eventos="Got character ZDOID from|Destroying abandoned non-persistent zdo|Got text|Game server connected"
-e ON_VALHEIM_LOG_FILTER_MATCHES_Eventos='{ read l; curl ... -d "{\"log\":\"$l\"}" ... }'
```

**1. `MATCHES` não existe.** Os prefixos do `valheim-server-docker` são
`VALHEIM_LOG_FILTER_MATCH` (casamento exato), `_STARTSWITH`, `_ENDSWITH`,
`_CONTAINS` e `_REGEXP`. Como o seu filtro usa `|` (alternância), o certo é
**`_REGEXP`** — com `MATCHES`, o hook simplesmente nunca dispara.

*(Também troquei `non-persistent` por `non.persistent`: dependendo da versão, o
servidor escreve com hífen ou com espaço, e o `.` do regex cobre as duas.)*

**2. JSON montado no shell quebra.** A linha de chat do Valheim contém aspas
(`Got text "vem pro barco" from Odin`). Interpolá-la em
`-d "{\"log\":\"$l\"}"` produz JSON inválido, e o evento se perde com um erro
que só aparece no log do container do jogo. Por isso a rota aceita a **linha
crua** com `Content-Type: text/plain` — sem escape, sem `read`, sem como
quebrar.

O formato JSON continua aceito, para quem já tem algo montado assim:

```jsonc
{"log": "03/24/2024 22:31:07: Got character ZDOID from Odin : 42:1"}
{"logs": ["linha 1", "linha 2"]}   // lote, até 100 linhas
```

### Conferir que está chegando

```bash
curl -i -X POST http://localhost:6060/eventos \
  -H "Content-Type: text/plain" \
  -H "X-Ingest-Token: SEU_TOKEN" \
  --data-binary '03/24/2024 22:31:07: Got character ZDOID from Odin : 42:1'
```

Responde `202` com o que foi entendido. O evento aparece no painel na hora.

```json
{"recebidos": 1, "ignorados": 0, "eventos": [{"tipo": "entrou", "jogador": "Odin", ...}]}
```

`ignorados` são linhas entendidas que **não** viraram evento novo. Elas
existem por causa de como o Valheim anuncia uma desconexão: ele escreve uma
linha `Destroying abandoned…` para cada objeto que a pessoa deixou no mundo —
dezenas, no mesmo segundo. O receptor transforma a primeira em "saiu" e conta
as outras aqui, em vez de encher o feed com a mesma notícia quarenta vezes.

---

## O painel

`http://localhost:6060/` — entre com o e-mail e a senha que o `iniciar.sh`
imprimiu.

| Tela | O que tem |
|---|---|
| **Painel** | quem está no mundo agora, números das últimas 24h e o feed ao vivo (SSE) |
| **Administração** | usuários, integração com o Discord e o log do processo |

Dois papéis: **administrador** (mexe em tudo) e **visualizador** (só o painel).
O administrador cria os outros em *Administração › Usuários*.

Perdeu a senha do único administrador? A porta dos fundos é a CLI, de dentro do
container:

```bash
docker compose exec app valheim-webhook usuario senha --email admin@valheim.local --senha uma-senha-nova
```

---

## Discord

Dá para configurar **pelo painel**, em *Administração › Integração com o
Discord* — o que você salvar ali passa a valer na hora, sem reiniciar. Dois
caminhos:

**Webhook de canal** (dois minutos): no Discord, *Editar canal › Integrações ›
Webhooks › Novo webhook*, copie a URL e cole no painel.

**App/bot** (o que cresce): crie o app em
[discord.com/developers](https://discord.com/developers/applications), pegue o
token do bot, convide-o no servidor com permissão de *Enviar mensagens*, e
preencha token + ID do canal no painel.

Com os dois preenchidos, o bot vence. O botão **Enviar mensagem de teste**
confirma o caminho inteiro.

Na tela também se escolhe **quais eventos viram mensagem**. O padrão deixa de
fora `conexao` (que vem colada com um `entrou` logo depois) e `desconhecido`
(que é diagnóstico do receptor, não notícia do servidor).

Duas proteções que já vêm ligadas, porque o conteúdo vem de quem joga:
menções desligadas (`@everyone` digitado no jogo não toca sino de ninguém) e
marcação do Discord escapada no texto do chat.

---

## Publicar num domínio

Exemplo: domínio-base `atila.cloud`, painel em `valheim.atila.cloud`.

No `.env`:

```bash
BASE_DOMAIN=atila.cloud            # aceita CORS de qualquer *.atila.cloud
PAINEL_DOMINIO=valheim.atila.cloud # endereço público do painel
COOKIE_SEGURO=true                 # cookie de sessão só em HTTPS
COOKIE_DOMINIO=                    # vazio = só valheim.atila.cloud (mais restrito)
TRUSTED_PROXY=172.16.0.0/12        # faixa do proxy, para o IP real no log
HTTP_PORT=6060
```

`COOKIE_DOMINIO` só precisa ser preenchido (com `.atila.cloud`) se a sessão
tiver de valer em **outros** subdomínios — não é o caso comum, e o valor vazio é
o mais seguro.

Com `PAINEL_DOMINIO` preenchido, o `./iniciar.sh` sobe junto um **Caddy** que
cuida do HTTPS sozinho (perfil `proxy` do compose, `deploy/Caddyfile`) —
certificado emitido e renovado sem comando nenhum. Requisitos: o domínio
resolvendo para esta máquina e as portas 80/443 livres.

Se preferir seu próprio proxy, suba sem o perfil (`docker compose up -d`) e
aponte-o para a porta publicada. Nos dois casos, use
`BIND_ADDR=172.17.0.1`: assim a porta 6060 fica visível para o container do
Valheim (`host.docker.internal`) e para o proxy, mas **não** para a internet.

<details>
<summary>nginx</summary>

```nginx
server {
    listen 443 ssl http2;
    server_name valheim.atila.cloud;

    ssl_certificate     /etc/letsencrypt/live/valheim.atila.cloud/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/valheim.atila.cloud/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:6060;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # O feed ao vivo é SSE: sem estas três linhas, o nginx segura os
        # eventos no buffer e o painel só atualiza em blocos.
        proxy_buffering off;
        proxy_read_timeout 24h;
        proxy_http_version 1.1;
    }
}
```
</details>

<details>
<summary>Caddy</summary>

```caddy
valheim.atila.cloud {
    reverse_proxy 127.0.0.1:6060 {
        flush_interval -1   # não bufferiza o SSE
    }
}
```
</details>

O servidor de Valheim continua entregando os eventos por
`http://host.docker.internal:6060/eventos` — pela rede interna, sem passar pelo
domínio.

---

## Configuração

Duas fontes, nessa ordem: o arquivo (`configs.json`, modelo em
`configs_example.json`) e as variáveis de ambiente, que vencem o arquivo. No
Docker, o arquivo vem na imagem (`configs.docker.json`) e tudo o que muda por
instalação chega pelo `.env`.

| Variável | Para quê |
|---|---|
| `VALHEIM_WEBHOOK_JWT_SECRET` | assina o cookie de sessão (vazio = sorteado a cada boot, e todo mundo é deslogado no reinício) |
| `VALHEIM_WEBHOOK_INGEST_TOKEN` | exigido no `X-Ingest-Token` do `POST /eventos` |
| `VALHEIM_WEBHOOK_ADMIN_EMAIL` / `_ADMIN_SENHA` | conta administrativa inicial (senha vazia = sorteada e impressa uma vez) |
| `VALHEIM_WEBHOOK_DISCORD_WEBHOOK_URL` | webhook de canal |
| `VALHEIM_WEBHOOK_DISCORD_BOT_TOKEN` / `_DISCORD_CHANNEL_ID` | app/bot |
| `VALHEIM_WEBHOOK_BASE_DOMAIN` / `_PAINEL_DOMINIO` | publicação em domínio |
| `VALHEIM_WEBHOOK_COOKIE_SEGURO` / `_COOKIE_DOMINIO` | cookie de sessão |
| `VALHEIM_WEBHOOK_TRUSTED_PROXY` / `_CORS_ORIGENS` | listas separadas por vírgula |
| `VALHEIM_WEBHOOK_PG_*` | banco (`HOST`, `PORT`, `USER`, `PWD`, `DB`, `SSLMODE`) |
| `VALHEIM_WEBHOOK_SERVIDOR` / `_PAINEL_TITULO` / `_HTTP_PORT` | identificação e rede |

Com `app.env=prod` (o padrão da imagem), subir sem `INGEST_TOKEN` ou sem
`JWT_SECRET` é **erro de boot**, não aviso: rota de ingestão aberta na internet
e sessão que morre a cada deploy são coisas que ninguém escolhe de propósito.

---

## API

Autenticação por cookie de sessão (`POST .../auth/login`) ou
`Authorization: Bearer <token>`.

| Método | Rota | Quem |
|---|---|---|
| `POST` | `/eventos` | o servidor de Valheim (token de ingestão) |
| `GET` | `/api/status` | qualquer um (health check) |
| `POST` | `/api/application/identidade/auth/login` | qualquer um |
| `GET` | `/api/application/identidade/auth/eu` | logado |
| `PATCH` | `/api/application/identidade/auth/eu/senha` | logado |
| `GET` | `/api/application/eventos/ingestao/stream` | logado (SSE) |
| `GET` | `/api/domain/eventos/eventos` | logado (filtros: `tipo`, `jogador`, `desde`, `ate`, `busca`, `page`, `pageSize`) |
| `GET` | `/api/domain/eventos/eventos/{uuid}` | logado |
| `GET` | `/api/domain/eventos/resumo` | logado (`?horas=24`, `0` = tudo) |
| `GET` | `/api/domain/mundo/jogadores` | logado (`?online=1`) |
| `GET` `PUT` `POST` | `/api/domain/configuracao/discord[/teste]` | administrador |
| `GET` | `/api/domain/observabilidade/logs` | administrador |
| `GET` `POST` `PATCH` `DELETE` | `/api/domain/identidade/usuarios[/{uuid}]` | administrador |

Todo erro sai no mesmo formato, com um `ray_trace` que também volta no header
`X-Request-Id` — é a chave para achar a requisição no log:

```json
{"code": 422, "error": "unprocessable_entity", "message": "Linha de log vazia.", "ray_trace": "5c2e…"}
```

---

## Desenvolvimento (sem Docker)

```bash
go mod tidy                               # o go.sum ainda não é versionado
cp configs_example.json configs.json      # ajuste o Postgres
docker compose up -d postgres             # ou um Postgres seu
go run . migrate up
go run . serve
```

O `configs_example.json` aponta para `localhost:5433`. O compose só publica a
porta do Postgres no perfil de desenvolvimento — para rodar o Go na máquina e o
banco em container, acrescente `ports: ["5433:5432"]` ao serviço `postgres`.

Antes de fechar uma alteração:

```bash
go build ./... && go vet ./... && go test ./...
```

`arch-go` (opcional, `go install github.com/arch-go/arch-go@latest`) valida as
regras de camada do `arch-go.yml`.

---

## Arquitetura

Monólito modular em Go, no mesmo desenho do projeto `atila`: DDD em duas
camadas de negócio, singleton por subdomínio, bootstrap por injeção de
dependência. O `AGENTS.md` tem as convenções; o resumo:

```
cmd/
├── cli/                 comandos: serve, migrate, usuario
├── bootstrap/           composição do processo (o único lugar que enxerga tudo)
└── server/routes/       engine do Gin, painel, health check
internal/
├── pkg/                 folhas: config, rest_err, pagination, reqctx, papel,
│                        errobserve, log/{access_log,memoria}, tempo_real
├── infra/               postgres, migrations, jwt, notificador/discord
├── iam/                 domain/identidade/usuario · application/identidade/auth
│                        middleware (cadeia de autorização)
└── valheim/
    ├── domain/          eventos/evento · mundo/jogador ·
    │                    configuracao/integracao · observabilidade/trilhas
    └── application/     eventos/ingestao (costura tudo)
web/                     painel embutido no binário (go:embed)
db/migrations/           SQL, com `down` para todo `up`
```

Regras de dependência (verificadas pelo `arch-go`): `pkg` não importa nada de
`internal` fora de `pkg`; `infra` só importa `pkg`; `domain` importa `pkg` e
`infra`, nunca um irmão; `application` orquestra por **interfaces declaradas
nela mesma** (`contratos.go`), ligadas em `cmd/bootstrap/adaptadores.go`.

Duas decisões que valem a leitura antes de mexer:

- **o diário e o saldo são tabelas separadas.** `eventos/evento` é o que
  aconteceu (uma linha por fato, nunca alterada); `mundo/jogador` é o estado
  atual (uma linha por pessoa, reescrita a cada fato). Perguntas diferentes,
  índices diferentes;
- **a ingestão tem uma ordem de importância.** Gravar o evento é a única etapa
  cuja falha vira erro para quem chamou. Atualizar o personagem, publicar no
  painel e avisar no Discord falham em silêncio (com log): o servidor de jogo
  não espera por ninguém.
