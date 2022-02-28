DROP TABLE IF EXISTS event_types;
CREATE TABLE event_types
(
    id         INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    name       VARCHAR(255),
    account_id INTEGER
);

DROP TABLE IF EXISTS languages;
CREATE TABLE languages
(
    id   INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    code VARCHAR(255)
);


DROP TABLE IF EXISTS deals;
CREATE TABLE deals
(
    id   INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255),
    deal_id varchar(255)
);

DROP TABLE IF EXISTS audiences;
CREATE TABLE audiences
(
    id   INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    info VARCHAR(255),
    info2 varchar(255)
);