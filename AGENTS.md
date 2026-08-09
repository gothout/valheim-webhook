# AGENTS.md — raiz do módulo `valheim-webhook`

Convenções que valem para qualquer alteração neste repositório. O README
explica o produto; este arquivo explica como mexer nele.

A arquitetura é a do projeto `atila` (monólito modular Go/DDD, singleton por
subdomínio, bootstrap por DI), reduzida ao tamanho deste problema. Onde há
diferença, ela está marcada como **[difere do atila]** com o motivo.

## Checks obrigatórios antes de fechar uma alteração

```bash
go mod tidy                                       # na primeira vez: gera o go.sum
go build ./... && go vet ./... && go test ./...
```

**O `go.sum` ainda não está versionado** e o `go.mod` lista só as dependências
diretas — o primeiro `go mod tidy` resolve as indiretas e grava as somas.
Comite os dois no mesmo commit e apague esta nota (e ajuste o `Dockerfile`, que
hoje roda `tidy` no build por causa disso).

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
