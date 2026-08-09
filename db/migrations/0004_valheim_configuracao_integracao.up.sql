-- A configuração da integração com o Discord, editável pelo painel.
--
-- A tabela é chaveada por SERVIÇO, e não é uma tabela de uma linha só, porque a
-- segunda integração (Telegram, um webhook genérico) não deve exigir migration
-- nova — só uma linha nova.
--
-- Os segredos ficam aqui em texto claro, e isso é uma decisão consciente: para
-- CIFRÁ-LOS seria preciso uma chave, que teria de ficar em algum lugar que o
-- processo alcança sozinho — ou seja, ao lado do banco. O que protege este dado
-- é o acesso ao Postgres, o mesmo que protege o resto. O que o produto garante
-- é que o segredo nunca SAI daqui: a API só devolve máscara.
CREATE TABLE IF NOT EXISTS valheim_configuracao_integracao (
    uuid           UUID PRIMARY KEY,
    servico        TEXT        NOT NULL,
    habilitado     BOOLEAN     NOT NULL DEFAULT FALSE,
    modo           TEXT        NOT NULL DEFAULT 'webhook',
    webhook_url    TEXT        NOT NULL DEFAULT '',
    bot_token      TEXT        NOT NULL DEFAULT '',
    canal_id       TEXT        NOT NULL DEFAULT '',
    username       TEXT        NOT NULL DEFAULT '',
    eventos        TEXT        NOT NULL DEFAULT '',
    atualizado_por UUID,
    criado_em      TIMESTAMPTZ NOT NULL DEFAULT now(),
    atualizado_em  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT valheim_configuracao_integracao_servico UNIQUE (servico),
    CONSTRAINT valheim_configuracao_integracao_modo_valido
        CHECK (modo IN ('bot', 'webhook'))
);
