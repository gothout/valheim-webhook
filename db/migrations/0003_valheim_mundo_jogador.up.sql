-- O SALDO: uma linha por personagem, reescrita a cada acontecimento.
--
-- A chave natural é `(servidor, nome)` — o nome do personagem é a única
-- identidade que o log oferece. O único sobre ela não é só integridade: é o que
-- faz o UPSERT da ingestão funcionar, e o que impede duas linhas de log
-- chegando juntas de criarem o mesmo personagem duas vezes.
CREATE TABLE IF NOT EXISTS valheim_mundo_jogador (
    uuid            UUID PRIMARY KEY,
    servidor        TEXT        NOT NULL,
    nome            TEXT        NOT NULL,
    online          BOOLEAN     NOT NULL DEFAULT FALSE,
    zdoid           TEXT        NOT NULL DEFAULT '',
    entrou_em       TIMESTAMPTZ,
    primeiro_em     TIMESTAMPTZ NOT NULL,
    ultimo_em       TIMESTAMPTZ NOT NULL,
    sessoes         BIGINT      NOT NULL DEFAULT 0,
    mortes          BIGINT      NOT NULL DEFAULT 0,
    -- Soma das sessões JÁ ENCERRADAS. A sessão em curso não entra aqui: quem
    -- exibe soma `now - entrou_em` na hora, e é por isso que este número nunca
    -- precisa de um job para ficar correto.
    tempo_total_seg BIGINT      NOT NULL DEFAULT 0,
    criado_em       TIMESTAMPTZ NOT NULL DEFAULT now(),
    atualizado_em   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT valheim_mundo_jogador_servidor_nome UNIQUE (servidor, nome)
);

-- A lista do painel: online primeiro, depois quem apareceu mais recentemente.
CREATE INDEX IF NOT EXISTS idx_jogador_listagem
    ON valheim_mundo_jogador (servidor, online DESC, ultimo_em DESC);

-- Quem está no mundo agora é dono de um ZDOID; quem saiu, não. O índice
-- parcial cobre exatamente a consulta da linha de saída.
CREATE INDEX IF NOT EXISTS idx_jogador_zdoid_online
    ON valheim_mundo_jogador (servidor, zdoid)
    WHERE online = TRUE AND zdoid <> '';
