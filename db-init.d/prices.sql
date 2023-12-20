CREATE TABLE IF NOT EXISTS prices (
    "id" bigint NOT NULL,
    "price" real NOT NULL,
    "timestamp" varchar(15) NOT NULL,
    "signatures" bytea ARRAY NULL,
    "inserted" timestamp NOT NULL,
    PRIMARY KEY ("id")
);
