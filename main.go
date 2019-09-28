package main

import (
	"bot/journal"
	"encoding/json"
	"io/ioutil"
	"os"
)

type config struct {
	TheMovieDBKey string `json:"themoviedb_key"`
	TelegramToken string `json:"telegram_token"`
	DBName        string `json:"db_name"`
}

func main() {
	journal.Info("application start")

	// Вычитывание настроек.
	journal.Info("reading config file")
	f, err := os.Open("config.json")
	if err != nil {
		journal.Fatal(err)
	}
	configRaw, err := ioutil.ReadAll(f)
	f.Close()
	if err != nil {
		journal.Fatal(err)
	}
	var cfg config
	err = json.Unmarshal(configRaw, &cfg)
	if err != nil {
		journal.Fatal(err)
	}
	journal.Info("config file read OK")

	// Горутина для пополнения БД фильмами.
	journal.Info("starting movies fetch goroutine")
	go theMovieDBCrawler(cfg.TheMovieDBKey, cfg.DBName)
	select {}
}
