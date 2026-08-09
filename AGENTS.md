# AGENTS.md — raiz do módulo `valheim-webhook`

Convenções que valem para qualquer alteração neste repositório. O README
explica o produto; este arquivo explica como mexer nele.

A arquitetura é a do projeto `atila` (monólito modular Go/DDD, singleton por
subdomínio, bootstrap por DI), reduzida ao tamanho deste problema. Onde há
diferença, ela está marcada como **[difere do atila]** com o motivo.

## Checks obrigatórios antes de fechar uma alteração

```bash
go build ./... && go vet ./... && go test ./...
```

**O toolchain é o Go 1.25**, mesmo com `go 1.24.5` no `go.mod`. Não é
preciosismo: uma dependência de TESTE transitiva (`rogpeppe/go-internal`, pela
cadeia do testify) exige `go >= 1.25`, e com 1.24 a resolução de módulos falha
antes de compilar. O `Dockerfile` usa `golang:1.25-alpine` pelo mesmo motivo.

Sem Go na máquina? O build acontece no container:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine \
  sh -c 'go build ./... && go vet ./... && go test ./...'
```

`arch-go` (`go install github.com/arch-go/arch-go@latest`) valida as regras de
camada do `arch-go.yml`. Ele exige **100% de cobertura**: pacote novo que
nenhuma regra descreve reprova, em vez de passar despercebido.

## Estrutura

```
main.go                  só delega para a CLI
cmd/cli/                 serve · migrate · usuario
cmd/bootstrap/           config → log → infra → migrations → domínios → HTTP
cmd/server/routes/       engine do Gin, painel, health check, CORS
internal/pkg/            folhas (não importam nada de internal fora de pkg)
internal/infra/          postgres, migrations, jwt, notificador/discord
internal/iam/            usuário, sessão e a cadeia de autorização
internal/valheim/        o produto
web/                     painel embutido (go:embed)
db/migrations/           SQL puro, `down` obrigatório
```

### Anatomia de um subdomínio

`internal/{sistema}/domain/{dominio}/{subdominio}/` tem estes arquivos:

| Arquivo | Responsabilidade |
|---|---|
| `model.go` | entidades GORM, tipos e constantes do domínio |
| `dto_request.go` / `dto_response.go` | entrada e saída da API |
| `errors.go` | sentinelas + o mapa `errCodes` |
| `repository.go` | interface + implementação Postgres |
| `service.go` | as regras |
| `controller.go` | handlers, rotas e a tradução sentinela → status |
| `singleton.go` | `New(deps)`, `Use()`, `MustUse()` |

Subdomínio de `application/` troca `model.go` e `repository.go` por
`contratos.go` (as interfaces dos vizinhos). Subdomínio que não persiste nada
também não tem `repository.go` — é o caso de `observabilidade/trilhas`, cuja
fonte é o anel de log em memória.

## Regras de dependência (invioláveis, verificadas pelo `arch-go`)

1. `pkg` não importa nada de `internal/` fora de `pkg`.
2. `infra` importa apenas `pkg` + libs externas — e **não importa outro pacote
   de `infra`**. Quem precisa do vizinho recebe a dependência pronta do
   `cmd/bootstrap` (é assim que o runner de migrations recebe a conexão).
3. `domain` importa `pkg` + `infra`. Nunca `application`, nunca um irmão.
4. `application` orquestra por **interfaces declaradas nela mesma**
   (`contratos.go`), ligadas em `cmd/bootstrap/adaptadores.go`. A regra 5 do
   atila permitiria importar `domain` direto; não fazemos — o ganho é
   `ingestao` rodar em teste sem banco, sem Discord e sem singleton.
5. `internal/iam/middleware` **não importa `iam/domain`**: os controllers de lá
   importam ELE. Tudo o que precisa do domínio entra por interface.
6. `cmd/server` não importa `infra`: o estado das dependências chega como
   `Sonda` injetada pelo boot.
7. `cmd` enxerga tudo; nada de `internal/` enxerga `cmd`.

## O que vale para todo subdomínio

- **auth é declarada rota a rota** pelo `Routes()` do controller, nunca no
  grupo. Rota pública nova não depende de alguém lembrar de excluí-la de um
  middleware global.
- **todo retorno de erro do service passa por `obs.Observe(ctx, err)`**, que
  devolve o erro intacto. Sentinela nova entra no `errCodes` no mesmo commit —
  fora do mapa, ela vira `ErrUnknown`.
- **o controller responde erro só por `rest_err.WriteError`**, traduzindo
  sentinela → status num `switch`. Erro não mapeado vira 500 genérico com a
  causa preservada para o observador.
- **`New` é idempotente** (`sync.Once`) e devolve o controller; `MustUse()`
  entra em pânico se não inicializado — e é isso que o `Routes()` usa.

## Decisões que já foram discutidas

**O diário e o saldo são tabelas separadas.** `eventos/evento` guarda o fato
(uma linha por acontecimento, nunca alterada, com o log CRU sempre presente);
`mundo/jogador` guarda o estado (uma linha por pessoa, reescrita a cada fato).
Responder "quem está online?" com a tabela de eventos, ou "o que houve às
22h31?" com a de personagens, dá trabalho e sai errado.

**Linha de log desconhecida é gravada, não descartada.** Formato de log de jogo
muda a cada atualização. O tipo `desconhecido` com a linha inteira é dado que se
reprocessa; a linha descartada não volta.

**A ingestão tem uma hierarquia de falhas.** Gravar o evento é a única etapa
cuja falha vira erro para quem chamou. Atualizar o personagem, publicar no
painel e notificar o Discord falham em silêncio (com log). O hook do container
tem um `curl` com prazo e o servidor de jogo não espera.

**Notificação só na TRANSIÇÃO de presença.** O servidor emite
`Got character ZDOID` também no renascimento; sem a trava, o canal receberia
"Fulano entrou" a cada morte.

**Uma saída é dezenas de linhas.** Quando alguém desconecta, o Valheim escreve
`Destroying abandoned non persistent zdo <dono>:<n>` para CADA objeto que a
pessoa deixou no mundo — 40+ no mesmo segundo, no log real deste projeto. O que
amarra a enxurrada a uma pessoa é o DONO (a metade antes do `:`), e o
`supressorDeSaidas` (memória, janela de 30s) deixa passar só a primeira. As
demais voltam como `ignorados` na resposta do `POST /eventos`, nunca como erro:
a linha não se perdeu, ela é a mesma notícia de novo.

Corolário que já mordeu: a busca do personagem por ZDOID casa pelo DONO, não
pelo ZDOID inteiro. O primeiro objeto destruído quase nunca é o do personagem —
que é justamente o único ZDOID que o registro guarda.

**O carimbo de hora não é ancorado no começo da linha.** Dependendo de como o
container encaminha o log, a linha chega crua ou com um prefixo de supervisor
pela frente. Ancorar fazia o evento usar a hora da CHEGADA em vez da hora do
JOGO — diferença invisível no dia a dia e errada exatamente quando importa
(receptor que ficou fora do ar e recebeu um lote atrasado).

**Segredo nunca sai do subdomínio que o guarda.** O hash de senha tem
`json:"-"` e a comparação é um método do service; os segredos do Discord só
saem mascarados (`••••1234`). Quem edita manda o valor novo — ou omite o campo,
que significa "mantenha".

**A configuração do Discord vive em dois lugares, e isso é de propósito:** o
`configs.json` é a SEMENTE (o que sobe numa instalação nova) e o banco é a
verdade a partir do primeiro salvamento pelo painel. Salvar reconfigura o
publicador na hora, sem reiniciar.

**O middleware confirma a conta no banco a cada requisição.** O token dura uma
semana; sem a conferência, remover ou rebaixar um usuário só passaria a valer no
próximo login dele. É uma leitura por chave primária — menos do que montar o
JSON da resposta.

## [difere do atila]

| Aqui | No atila | Por quê |
|---|---|---|
| Sem multi-tenancy (`internal/pkg/reqctx` só carrega ray_trace e identidade) | `tenantctx` com workspace/tenant em toda query | um receptor de eventos de UM servidor não tem escopo para carregar |
| Config por `encoding/json` + env explícito | viper | o arquivo é JSON e o que precisa vir de fora são três segredos; a resolução implícita do viper ignora chave não registrada em silêncio |
| Sem Swagger gerado | `swag init` obrigatório, `docs/` versionado | as anotações `@Summary` estão nos controllers e o `swag` roda quando alguém quiser; sem `docs/` gerado, importá-lo quebraria o build |
| Sem ClickHouse: `errobserve` cai no slog e o painel lê um anel em memória | trilhas de log no ClickHouse | um segundo banco para um serviço deste tamanho é custo sem retorno |
| `WriteTimeout: 0` no servidor HTTP | 120s | o feed do painel é SSE: uma conexão aberta por horas seria derrubada a cada prazo |
| Papéis (`admin`/`visualizador`) em `internal/pkg/papel` | RBAC granular por permissão | duas decisões (mexe / olha), não vinte |

## Migrations

- SQL puro em `db/migrations`, `NNNN_{sistema}_{dominio}_{subdominio}.{up,down}.sql`.
- **Todo `up` tem `down`** — o `migrate validate` reprova sem ele, e roda sem
  banco (é o que entra no CI).
- Migration aplicada nunca é editada: correção é migration nova.
- Nunca `AutoMigrate` do GORM.

## Idioma

Comentários, documentação e mensagens de erro em **PT-BR**; identificadores de
código em inglês/snake_case conforme os templates. Comentário explica **por
quê**, não o que a linha já diz.
