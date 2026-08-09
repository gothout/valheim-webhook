-- O DIÁRIO: uma linha por acontecimento no servidor de Valheim.
--
-- `linha` guarda o log CRU, sempre — inclusive quando o parser não entendeu
-- (tipo `desconhecido`). Formato de log de jogo muda a cada atualização, e uma
-- linha guardada inteira é dado que se reprocessa depois; uma linha descartada
-- porque o regex não casou é dado que não volta nunca.
CREATE TABLE IF NOT EXISTS valheim_eventos_evento (
    uuid        UUID PRIMARY KEY,
    servidor    TEXT        NOT NULL,
    tipo        TEXT        NOT NULL,
    jogador     TEXT        NOT NULL DEFAULT '',
    steam_id    TEXT        NOT NULL DEFAULT '',
    zdoid       TEXT        NOT NULL DEFAULT '',
    texto       TEXT        NOT NULL DEFAULT '',
    linha       TEXT        NOT NULL,
    ocorrido_em TIMESTAMPTZ NOT NULL,
    criado_em   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- O feed do painel: "os últimos N eventos". O desempate por `criado_em` está no
-- índice porque está na consulta — o carimbo do servidor de Valheim tem
-- resolução de SEGUNDO, e uma partida inteira entrando junto produz vários
-- eventos no mesmo segundo.
CREATE INDEX IF NOT EXISTS idx_evento_recentes
    ON valheim_eventos_evento (ocorrido_em DESC, criado_em DESC);

-- O resumo por tipo e o filtro do painel.
CREATE INDEX IF NOT EXISTS idx_evento_tipo_recentes
    ON valheim_eventos_evento (tipo, ocorrido_em DESC);

-- "O que este personagem andou fazendo?" — parcial porque metade dos eventos
-- (conexão, servidor pronto) não tem dono, e indexá-los seria indexar vazio.
CREATE INDEX IF NOT EXISTS idx_evento_jogador
    ON valheim_eventos_evento (jogador, ocorrido_em DESC)
    WHERE jogador <> '';

-- A ponte entre a saída e a entrada: a linha `Destroying abandoned…` traz o
-- ZDOID e não traz o nome, e é esta consulta que descobre de quem ele era.
CREATE INDEX IF NOT EXISTS idx_evento_zdoid
    ON valheim_eventos_evento (zdoid, ocorrido_em DESC)
    WHERE zdoid <> '';
