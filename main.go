package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
)

type config struct {
	ThemoviedbKey string `json:"themoviedb_key"`
	TelegramToken string `json:"telegram_token"`
}

func main() {
	// Вычитывание настроек.
	f, err := os.Open("config.json")
	if err != nil {
		log.Fatal(err)
	}
	configRaw, err := ioutil.ReadAll(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}
	var cfg config
	err = json.Unmarshal(configRaw, &cfg)
	if err != nil {
		log.Fatal(err)
	}

	go tmdbCrawler(cfg.ThemoviedbKey)
	select {}
}
