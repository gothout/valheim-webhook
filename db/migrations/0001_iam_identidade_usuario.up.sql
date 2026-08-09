-- Contas do painel.
--
-- O e-mail é a identidade de login, e o índice único é sobre `LOWER(email)`:
-- quem se cadastrou como `Thor@casa.com` vai digitar `thor@casa.com` na segunda
-- vez, e um único sobre a coluna crua deixaria as duas coexistirem.
CREATE TABLE IF NOT EXISTS iam_identidade_usuario (
    uuid             UUID PRIMARY KEY,
    nome             TEXT        NOT NULL,
    email            TEXT        NOT NULL,
    senha_hash       TEXT        NOT NULL,
    papel            TEXT        NOT NULL,
    ativo            BOOLEAN     NOT NULL DEFAULT TRUE,
    ultimo_acesso_em TIMESTAMPTZ,
    criado_em        TIMESTAMPTZ NOT NULL DEFAULT now(),
    atualizado_em    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT iam_identidade_usuario_papel_valido
        CHECK (papel IN ('admin', 'visualizador'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_iam_usuario_email
    ON iam_identidade_usuario (LOWER(email));

-- Índice parcial: a única consulta que percorre papéis é "quantos
-- administradores ativos restam?", que sustenta a regra do último admin.
CREATE INDEX IF NOT EXISTS idx_iam_usuario_admin_ativo
    ON iam_identidade_usuario (papel)
    WHERE ativo = TRUE;
