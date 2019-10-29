package main

import (
	"bot/journal"
	"encoding/json"
	"io/ioutil"
	"os"
)

func main() {
	journal.Info("application started")

	cfg, err := readConfig("config.json")
	if err != nil {
		journal.Fatal(err)
	}

	// Горутина для пополнения БД фильмами по The MovieDB API (api.themoviedb.org).
	go theMovieDBHarvester(cfg.TheMovieDBKey, cfg.DBName)

	// Горутина бота - взаимодействие по Telegram Bot API с пользователями Telegram.
	go bot(&cfg.Bot, cfg.DBName)

	select {}
}

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
