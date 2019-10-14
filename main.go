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
	journal.Info("application started")

	cfg, err := readConfig("config.json")
	if err != nil {
		journal.Fatal(err)
	}

	// Горутина для пополнения БД фильмами.
	journal.Info("starting movies fetch goroutine")
	go theMovieDBHarvester(cfg.TheMovieDBKey, cfg.DBName)
	select {}
}

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
