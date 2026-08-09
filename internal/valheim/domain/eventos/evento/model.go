// Package evento é o subdomínio que recebe, interpreta e guarda o que acontece
// no servidor de Valheim.
//
// A entrada é UMA LINHA de log, entregue pelo hook do container
// (`valheim-server-docker`) no `POST /eventos`. A saída é um registro tipado:
// quem entrou, quem morreu, quem falou, quem saiu.
//
// Entidade: Evento. Dependências: Postgres (repository).
//
// A decisão que organiza o pacote inteiro: **a linha crua é sempre guardada**,
// tenha o parser entendido a linha ou não. Formato de log de jogo muda a cada
// atualização, e um evento gravado como `desconhecido` com a linha inteira é
// dado que se reprocessa depois; uma linha descartada porque o regex não casou
// é dado que não volta nunca.
package evento

import (
	"time"

	"github.com/google/uuid"
)

// Identidade do subdomínio (observador de erros e logs de boot).
const (
	Sistema    = "valheim"
	Dominio    = "eventos"
	Subdominio = "evento"
)

// Tipo é o vocabulário fechado dos eventos reconhecidos.
//
// É um tipo nomeado, e não `string` solta, porque ele atravessa a fronteira do
// pacote em três lugares (o filtro da listagem, a configuração do Discord e o
// painel) e um valor digitado errado precisa parar no compilador ou na
// validação, não virar filtro que não casa com nada.
type Tipo string

const (
	// TipoConexao — `Got connection SteamID 7656…`. O cliente apareceu; o
	// personagem ainda não entrou no mundo.
	TipoConexao Tipo = "conexao"
	// TipoEntrou — `Got character ZDOID from Fulano : 123:1`. O personagem
	// nasceu no mundo. É também o que sai num respawn.
	TipoEntrou Tipo = "entrou"
	// TipoSaiu — `Destroying abandoned non persistent zdo 123:1`. O servidor
	// está limpando o que o jogador deixou para trás: ele caiu ou desconectou.
	TipoSaiu Tipo = "saiu"
	// TipoMorreu — `Got character ZDOID from Fulano : 0:0`. O ZDOID zerado é a
	// forma que o servidor tem de dizer que o personagem morreu.
	TipoMorreu Tipo = "morreu"
	// TipoMensagem — as linhas `Got text`: fala, grito e placa.
	TipoMensagem Tipo = "mensagem"
	// TipoServidorPronto — `Game server connected`. O mundo subiu e aceita
	// jogadores.
	TipoServidorPronto Tipo = "servidor_pronto"
	// TipoDesconhecido é a linha que passou pelo filtro do container mas não
	// casou com nenhum padrão conhecido. Guardada inteira, de propósito.
	TipoDesconhecido Tipo = "desconhecido"
)

// TiposConhecidos lista o vocabulário completo (validação de filtro e painel).
func TiposConhecidos() []Tipo {
	return []Tipo{
		TipoConexao, TipoEntrou, TipoSaiu, TipoMorreu,
		TipoMensagem, TipoServidorPronto, TipoDesconhecido,
	}
}

// TiposNotificaveisPadrao é o que vai para o Discord quando
// `discord.eventos` não é preenchido.
//
// Fora da lista ficam `conexao` (que sempre vem colada com um `entrou` logo
// depois — anunciar os dois é anunciar duas vezes) e `desconhecido` (que é
// diagnóstico do receptor, não notícia do servidor).
func TiposNotificaveisPadrao() []Tipo {
	return []Tipo{TipoEntrou, TipoSaiu, TipoMorreu, TipoMensagem, TipoServidorPronto}
}

// Valido diz se o tipo pertence ao vocabulário.
func (t Tipo) Valido() bool {
	for _, conhecido := range TiposConhecidos() {
		if t == conhecido {
			return true
		}
	}
	return false
}

// Rotulo é o tipo em português, para o painel e para o Discord.
func (t Tipo) Rotulo() string {
	switch t {
	case TipoConexao:
		return "conexão"
	case TipoEntrou:
		return "entrou"
	case TipoSaiu:
		return "saiu"
	case TipoMorreu:
		return "morreu"
	case TipoMensagem:
		return "mensagem"
	case TipoServidorPronto:
		return "servidor pronto"
	default:
		return "desconhecido"
	}
}

// Evento é a entidade raiz do subdomínio: uma linha de log já interpretada.
type Evento struct {
	UUID uuid.UUID `gorm:"column:uuid;type:uuid;primaryKey" json:"uuid"`
	// Servidor é o nome do mundo (o `SERVER_NAME` do container). Existe para o
	// dia em que dois servidores apontarem para o mesmo receptor.
	Servidor string `gorm:"column:servidor;not null" json:"servidor"`
	Tipo     Tipo   `gorm:"column:tipo;not null" json:"tipo"`
	// Jogador é o nome do personagem. Vazio nos eventos que não têm dono
	// (`servidor_pronto`) e nos que o servidor não nomeia (`conexao`).
	Jogador string `gorm:"column:jogador;not null;default:''" json:"jogador,omitempty"`
	// SteamID só aparece na linha de conexão.
	SteamID string `gorm:"column:steam_id;not null;default:''" json:"steam_id,omitempty"`
	// ZDOID é o identificador do objeto do personagem no mundo (`123:1`). É a
	// única ponte entre o `entrou` (que traz o nome) e o `saiu` (que não traz).
	ZDOID string `gorm:"column:zdoid;not null;default:''" json:"zdoid,omitempty"`
	// Texto é o conteúdo da fala, nas linhas de mensagem.
	Texto string `gorm:"column:texto;not null;default:''" json:"texto,omitempty"`
	// Linha é o log cru, exatamente como chegou. Nunca é vazio.
	Linha string `gorm:"column:linha;not null" json:"linha"`
	// OcorridoEm é a hora carimbada pelo servidor de Valheim, convertida para
	// UTC. Sem carimbo na linha, é a hora da chegada.
	OcorridoEm time.Time `gorm:"column:ocorrido_em;not null" json:"ocorrido_em"`
	// CriadoEm é a hora em que o receptor gravou. A diferença entre as duas
	// mede o atraso do caminho container → webhook.
	CriadoEm time.Time `gorm:"column:criado_em;autoCreateTime" json:"criado_em"`
}

// TableName fixa o nome da tabela no padrão `{sistema}_{dominio}_{subdominio}`.
func (Evento) TableName() string { return "valheim_eventos_evento" }

// ListFilter são os filtros da listagem. Campo zerado não filtra.
type ListFilter struct {
	Tipos   []Tipo
	Jogador string
	Desde   *time.Time
	Ate     *time.Time
	// Busca casa com pedaço do texto da mensagem ou da linha crua.
	Busca string
}

// Resumo é a fotografia do servidor para o cabeçalho do painel.
//
// `PorTipo` traz TODOS os tipos conhecidos, inclusive os zerados: um painel que
// esconde o contador de mortes enquanto ninguém morreu muda de layout sozinho
// no pior momento.
type Resumo struct {
	Total        int64          `json:"total"`
	PorTipo      map[Tipo]int64 `json:"por_tipo"`
	UltimoEvento *time.Time     `json:"ultimo_evento,omitempty"`
}
