-- +goose Up
CREATE TABLE listed_info (
    code        text        NOT NULL,
    date        date        NOT NULL,
    co_name     text,
    co_name_en  text,
    s17         text,
    s17_nm      text,
    s33         text,
    s33_nm      text,
    scale_cat   text,
    mkt         text,
    mkt_nm      text,
    mrgn        text,
    mrgn_nm     text,
    prod_cat    text,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (code, date)
);

CREATE TABLE prices (
    code         text        NOT NULL,
    date         date        NOT NULL,
    open         numeric,
    high         numeric,
    low          numeric,
    close        numeric,
    volume       numeric,
    turnover     numeric,
    adj_factor   numeric,
    adj_open     numeric,
    adj_high     numeric,
    adj_low      numeric,
    adj_close    numeric,
    adj_volume   numeric,
    upper_limit  boolean,
    lower_limit  boolean,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (code, date)
);

CREATE INDEX prices_date_idx ON prices (date);

-- +goose Down
DROP TABLE prices;
DROP TABLE listed_info;
