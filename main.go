package main

import (
	"bot/journal"
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
