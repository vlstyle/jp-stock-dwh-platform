-- +goose Up
CREATE TABLE edinet_documents (
    doc_id          text        PRIMARY KEY,
    code            text        NOT NULL,
    filer_name      text,
    doc_type_code   text        NOT NULL,
    period_end      date,
    submit_date     date        NOT NULL,
    accounting_std  text,
    scope           text        NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX edinet_documents_code_idx ON edinet_documents (code, period_end DESC);

-- 有価証券報告書の当期末の貸借対照表から取り出した、現金・有価証券・有利子負債に関する項目（円）。
CREATE TABLE edinet_facts (
    doc_id      text    NOT NULL REFERENCES edinet_documents (doc_id) ON DELETE CASCADE,
    element_id  text    NOT NULL,
    value       numeric NOT NULL,
    PRIMARY KEY (doc_id, element_id)
);

-- +goose Down
DROP TABLE edinet_facts;
DROP TABLE edinet_documents;
