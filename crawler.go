package main

import (
	"bot/sqlite"
	"bot/themoviedb"
	"log"
)

func init() {
	// Инициализация БД.

	// Основная таблица с информацией о фильме.
	query := `
CREATE TABLE IF NOT EXISTS movie (
    id             INTEGER PRIMARY KEY,
    tmdb_id        INTEGER NOT NULL,
    original_title TEXT    NOT NULL,
    original_lang  TEXT    NOT NULL,
    release_date   TEXT    NOT NULL,
    adult          INTEGER NOT NULL,
    imdb_id        INTEGER,
    popularity     REAL);
`
	con, err := sqlite.NewConn("movie.db")
	if err != nil {
		log.Fatal(err)
	}

	ok, err := con.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Fatal("cannot create movie table")
	}

	// Таблица с дополнительной информацией о фильме из талбицы movie.
	query = `
CREATE TABLE IF NOT EXISTS movie_detail (
    id          INTEGER PRIMARY KEY,
    fk_movie_id REFERENCES movie(id) NOT NULL,
    lang        TEXT NOT NULL,
    title       TEXT NOT NULL,
    poster      BLOB);
);
`
	ok, err = con.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Fatal("cannot create movie_detail table")
	}

	// Таблица для хранения результата получения информации о фильме.
	query = `
CREATE TABLE IF NOT EXISTS movie_fetch (
    id      INTEGER PRIMARY KEY,
    tmdb_id INTEGER NOT NULL,
    -- Равенство result 0 означает, что вся информация о фильме получена.
    -- Другие значения обозначают количество неудачных попыток получения данных о фильме.
    result  INTEGER NOT NULL
);
`
	ok, err = con.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Fatal("cannot create movie_fetch table")
	}

	con.Close()
}

// tmdbCrawler заполняет локальную базу фильмов, скачивая информацию о них по
// The MovieDB API.
func tmdbCrawler(key string) {
	// TODO: необходимо передавать более тонко настроенный http.Client с не
	// бесконечным временем ожидания данных от сервера.
	tmdb := themoviedb.NewClient(key, nil)
	err := tmdb.Configure()
	if err != nil {
		log.Fatal(err)
	}
}
