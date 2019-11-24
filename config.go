package main

import (
	"bot/journal"
	"bot/sqlite"
	"encoding/json"
	"io/ioutil"
	"os"
)

type botConfig struct {
	Token       string `json:"telegram_token"`
	WebhookAddr string `json:"webhook_address"`
	WebhookPort int    `json:"webhook_port"`
	BotAPIAddr  string `json:"telegram_bot_api_address"`
	PublicCert  string `json:"public_cert"`
	PrivateKey  string `json:"private_key"`
}

type config struct {
	TheMovieDBKey string    `json:"themoviedb_key"`
	DBName        string    `json:"db_name"`
	Bot           botConfig `json:"bot_config"`
}

// Чтение настроек из файла настроек.
func readConfig(cfgFilename string) (*config, error) {
	journal.Info("reading config file")

	f, err := os.Open(cfgFilename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	configRaw, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var cfg config
	err = json.Unmarshal(configRaw, &cfg)
	if err != nil {
		return nil, err
	}

	journal.Info("config file read OK")
	return &cfg, nil
}

// Инициализация БД фильмов.
func initDB(dbName string) error {
	journal.Info(" initialising database " + dbName)

	con, err := sqlite.NewConn(dbName)
	if err != nil {
		return err
	}
	defer con.Close()
	journal.Trace("connected to " + dbName)

	//- Основная таблица с информацией о фильме.
	query := `
CREATE TABLE IF NOT EXISTS movie (
    id             INTEGER PRIMARY KEY,
    tmdb_id        INTEGER NOT NULL UNIQUE,
    original_title TEXT    NOT NULL,
    original_lang  TEXT    NOT NULL,
    released_on    TEXT    NOT NULL,
    adult          INTEGER NOT NULL,
    imdb_id        INTEGER,
    vote_count     INTEGER,
    vote_average   REAL,
    collection_id  INTEGER, -- Если равен 0, то фильм не принадлежит никакой коллекции.
    created_on     TEXT DEFAULT (datetime('now')),
    updated_on     TEXT
);
`
	_, err = con.Exec(query)
	if err != nil {
		return err
	}
	journal.Trace("table movie create OK")

	//- Таблица с дополнительной информацией о фильме из таблицы movie.
	query = `
CREATE TABLE IF NOT EXISTS movie_detail (
    id          INTEGER PRIMARY KEY,
    fk_movie_id REFERENCES movie(id) NOT NULL,
    lang        TEXT NOT NULL,
    title       TEXT NOT NULL,
    poster      BLOB,
    created_on  TEXT DEFAULT (datetime('now')),
    updated_on  TEXT,
                UNIQUE (fk_movie_id, lang)
);
`
	_, err = con.Exec(query)
	if err != nil {
		return err
	}
	journal.Trace("table movie_detail create OK")

	journal.Info("database " + dbName + " init OK")

	return nil
}
