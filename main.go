package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/source-farm/movie-promo-bot/journal"
)

func main() {
	journal.Info("application started")

	cfg, err := readConfig("config.json")
	if err != nil {
		journal.Fatal(err)
	}

	err = initDB(cfg.DBName)
	if err != nil {
		journal.Fatal(err)
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	// Горутина для пополнения БД фильмами по The MovieDB API (api.themoviedb.org).
	wg.Add(1)
	go theMovieDBHarvester(cancelCtx, &wg, cfg.TheMovieDBKey, cfg.DBName)

	// Горутина бота - взаимодействие по Telegram Bot API с пользователями Telegram.
	wg.Add(1)
	go bot(cancelCtx, &wg, cfg.Bot, cfg.DBName)

	// Выходим при получении какого-либо сигнала закрытия программы.
	quitSignal := make(chan os.Signal, 1)
	signal.Notify(quitSignal, syscall.SIGINT, syscall.SIGTERM)
	<-quitSignal
	cancel()
	wg.Wait()
	journal.Info("application finished")
	journal.Stop()
}
