package main

import (
	"bot/iso6391"
	"bot/journal"
	"bot/sqlite"
	"bot/themoviedb"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ruAlphabet         = "абвгдеёжзийклмнопрстуфхцчшщъыьэюяАБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ"
	enAlphabet         = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	enMovieMinRank     = 1.5
	crawlersNum        = 3
	movieFetchMaxFails = 3
	tmdbMaxRetries     = 3
	dbBusyTimeoutMS    = 10000
	httpReqTimeout     = time.Second * 15

	movieInsertQuery = `
INSERT INTO movie (tmdb_id, original_title, original_lang, release_date, adult, imdb_id, popularity)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
ON CONFLICT (tmdb_id) DO NOTHING;
`

	movieIDQuery = `
SELECT id
  FROM movie
 WHERE tmdb_id = ?1;
`

	posterInsertQuery = `
INSERT INTO movie_detail (fk_movie_id, lang, title, poster)
VALUES (?1, ?2, ?3, ?4);
`

	movieFetchSuccessQuery = `
INSERT INTO movie_fetch (tmdb_id, complete)
VALUES (?1, 1)
ON CONFLICT (tmdb_id) DO UPDATE SET complete = 1, updated_on = datetime('now');
`

	movieFetchFailQuery = `
INSERT INTO movie_fetch (tmdb_id, fail)
VALUES (?1, 1)
ON CONFLICT (tmdb_id) DO UPDATE SET fail = fail + 1, updated_on = datetime('now');
`

	movieFetchResultQuery = `
SELECT complete, fail
  FROM movie_fetch
 WHERE tmdb_id = ?1;
`
)

// Краткая информация о фильме из файла ежедневного экспорта The MovieDB API.
type movieBrief struct {
	ID         int     `json:"id"`
	Title      string  `json:"original_title"`
	Popularity float64 `json:"popularity"`
}

// theMovieDBCrawler заполняет локальную базу фильмов, скачивая информацию о
// них по The MovieDB API.
func theMovieDBCrawler(key, dbName string) {
	initDB(dbName)

	httpClient := &http.Client{
		Timeout: httpReqTimeout,
	}
	tmdb := themoviedb.NewClient(key, httpClient)
	err := tmdb.Configure()
	if err != nil {
		journal.Fatal(err)
	}

	movieIDs := make(chan int)
	go findNewMovies(tmdb, dbName, movieIDs)
	for i := 0; i < crawlersNum; i++ {
		go crawler(tmdb, dbName, movieIDs)
	}
}

// Инициализация БД фильмов.
func initDB(dbName string) {
	journal.Info("initialising database " + dbName)

	con, err := sqlite.NewConn(dbName)
	if err != nil {
		journal.Fatal(err)
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
    release_date   TEXT    NOT NULL,
    adult          INTEGER NOT NULL,
    imdb_id        INTEGER,
    popularity     REAL,
    created_on     TEXT DEFAULT (datetime('now')),
    updated_on     TEXT
);
`
	_, err = con.Exec(query)
	if err != nil {
		journal.Fatal(err)
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
    updated_on  TEXT
);
`
	_, err = con.Exec(query)
	if err != nil {
		journal.Fatal(err)
	}
	journal.Trace("table movie_detail create OK")

	//- Таблица для хранения результата получения информации о фильме.
	query = `
CREATE TABLE IF NOT EXISTS movie_fetch (
    id         INTEGER PRIMARY KEY,
    tmdb_id    INTEGER NOT NULL UNIQUE,
    complete   INTEGER DEFAULT 0, -- Если равен 1, то вся информация о фильме получена.
    fail       INTEGER DEFAULT 0, -- Количество неудачных попыток получения информации о фильме.
    created_on TEXT DEFAULT (datetime('now')),
    updated_on TEXT
);
`
	_, err = con.Exec(query)
	if err != nil {
		journal.Fatal(err)
	}
	journal.Trace("table movie_fetch create OK")

	journal.Info("database " + dbName + " init OK")
}

// findNewMovies записывает в канал movieIDs идентификаторы новых фильмов.
func findNewMovies(client *themoviedb.Client, dbName string, movieIDs chan<- int) {
	daily := "daily"

	// Очистка.
	defer func() {
		if _, err := os.Stat(daily); err == nil {
			os.Remove(daily)
		}
		close(movieIDs)
	}()

	// Скачиваем базу с краткой информацией о всех фильмах за какой-либо из
	// пяти предыдущих дней.
	var err error
	now := time.Now()
	for i := 1; i <= 5; i++ {
		date := now.AddDate(0, 0, -i)
		journal.Info("downloading daily export for " + date.Format("2006-01-02"))
		year, month, day := date.Date()
		err = client.GetDailyExport(year, int(month), day, daily)
		if err == nil {
			journal.Info("daily export for " + date.Format("2006-01-02") + " download OK")
			break
		} else {
			journal.Error("daily export for "+date.Format("2006-01-02")+" download fail: ", err)
		}
	}
	if err != nil {
		journal.Error("cannot download daily export for any of 5 previous days")
		return
	}

	// Полученный файл - это архив gzip. Извлекаем из него данные на лету.
	f, err := os.Open(daily)
	if err != nil {
		journal.Error(err)
		return
	}
	defer f.Close()
	uncompressed, err := gzip.NewReader(f)
	if err != nil {
		journal.Error(err)
		return
	}
	defer uncompressed.Close()

	// Установка связи с БД, подготовка запросов.
	conn, err := sqlite.NewConn(dbName)
	if err != nil {
		journal.Error(err)
		return
	}
	defer conn.Close()
	err = conn.SetBusyTimeout(dbBusyTimeoutMS)
	if err != nil {
		journal.Error(err)
		return
	}
	fetchResultStmt, err := conn.Prepare(movieFetchResultQuery)
	if err != nil {
		journal.Error(err)
		return
	}
	defer fetchResultStmt.Close()

	scanner := bufio.NewScanner(uncompressed)
mainLoop:
	for scanner.Scan() {
		movieRaw := scanner.Text()
		var movie movieBrief
		err = json.Unmarshal([]byte(movieRaw), &movie)
		if err != nil || movie.ID == 0 || movie.Title == "" {
			continue
		}

		var complete, failsNum int64
	dbBusyLoop:
		for {
			err = fetchResultStmt.QueryRow(movie.ID).Scan(&complete, &failsNum)
			switch err {
			case nil:
				fallthrough

			case sqlite.ErrNoRows:
				break dbBusyLoop

			case sqlite.ErrBusy:
				fallthrough

			case sqlite.ErrLocked:
				journal.Error(err, ", sleeping for 1 sec")
				time.Sleep(time.Second)

			default:
				journal.Error(err)
				continue mainLoop
			}
		}

		if complete == 1 {
			journal.Trace("movie [", movie.ID, "] is already fetched, skipping")
			continue
		} else if failsNum >= movieFetchMaxFails {
			journal.Info("movie [", movie.ID, "] has to many fetch fails, skipping")
			continue
		}

		switch {
		case strings.ContainsAny(movie.Title, ruAlphabet):
			fallthrough

		case strings.ContainsAny(movie.Title, enAlphabet) && movie.Popularity > enMovieMinRank:
			movieIDs <- movie.ID
		}
	}
	err = scanner.Err()
	if err != nil {
		journal.Error(err)
	}
}

// crawler - это одна горутина для скачивания информации о фильме по
// The MovieDB API и записи этой информации в БД.
func crawler(client *themoviedb.Client, dbName string, movieIDs <-chan int) {
	crawlerID := "[go " + genRandString(5) + "]"

	// Установка соединения с БД и её настройка.
	conn, err := sqlite.NewConn(dbName)
	if err != nil {
		journal.Error(err)
		return
	}
	defer conn.Close()
	journal.Info(crawlerID, " connected to database"+dbName)

	err = conn.SetBusyTimeout(dbBusyTimeoutMS)
	if err != nil {
		journal.Error(err)
		return
	}
	journal.Trace(crawlerID, " set database connection busy timeout to ", dbBusyTimeoutMS, " ms")

	// Подготавливаем запросы.
	movieInsertStmt, err := conn.Prepare(movieInsertQuery)
	if err != nil {
		journal.Fatal(err)
		return
	}
	defer movieInsertStmt.Close()
	journal.Trace(crawlerID, " movie insert query prepared")

	movieIDStmt, err := conn.Prepare(movieIDQuery)
	if err != nil {
		journal.Fatal(err)
		return
	}
	defer movieIDStmt.Close()
	journal.Trace(crawlerID, " movie id query prepared")

	posterInsertStmt, err := conn.Prepare(posterInsertQuery)
	if err != nil {
		journal.Fatal(err)
		return
	}
	defer posterInsertStmt.Close()
	journal.Trace(crawlerID, " poster insert query prepared")

	movieFetchFailStmt, err := conn.Prepare(movieFetchFailQuery)
	if err != nil {
		journal.Fatal(err)
		return
	}
	defer movieFetchFailStmt.Close()
	journal.Trace(crawlerID, " movie fetch fail query prepared")

	movieFetchSuccessStmt, err := conn.Prepare(movieFetchSuccessQuery)
	if err != nil {
		journal.Fatal(err)
		return
	}
	defer movieFetchSuccessStmt.Close()
	journal.Trace(crawlerID, " movie fetch success query prepared")

	rateLimitStr := strconv.Itoa(int(themoviedb.APIRateLimitDur.Seconds()))
	// Закачиваем фильмы.
mainLoop:
	for id := range movieIDs {
		var movie themoviedb.Movie
		var err error
		// Получаем общие данные фильма.
	movieFetchLoop:
		for i := 0; i < tmdbMaxRetries; i++ {
			journal.Trace(crawlerID, " fetching movie [", id, "]")
			movie, err = client.GetMovie(id)
			switch err {
			case nil:
				journal.Info(crawlerID, " movie [", id, "] fetch OK")
				break movieFetchLoop

			case themoviedb.ErrRateLimit:
				if i == (tmdbMaxRetries - 1) {
					journal.Error(crawlerID, " movie [", id, "] fetch fail")
				} else {
					journal.Info(crawlerID, " tmdb rate limit exceeded, sleeping for "+rateLimitStr+" sec")
					time.Sleep(themoviedb.APIRateLimitDur)
				}

			default:
				journal.Error(crawlerID, " movie [", id, "] fetch error: ", err)
				break movieFetchLoop
			}
		}
		if err != nil {
			_, err = movieFetchFailStmt.Exec(id)
			if err != nil {
				journal.Error(crawlerID, " ", err)
			}
			continue
		}

		// Закачиваем постеры фильма.
		type posterData struct {
			image []byte
			lang  iso6391.LangCode
			title string
		}
		var posters []posterData
		err = nil
	posterLoop:
		for _, poster := range movie.Poster {
			// Если нет названия фильма на том же языке, что и постер, то не
			// скачиваем постер.
			if _, ok := movie.Title[poster.Lang]; !ok {
				continue
			}
			var image []byte

		posterFetchLoop:
			for i := 0; i < tmdbMaxRetries; i++ {
				journal.Trace(crawlerID, " fetching movie [", id, "] poster ("+poster.Lang+")")
				image, err = client.GetPoster(poster.Path)
				switch err {
				case nil:
					journal.Info(crawlerID, " movie [", id, "] poster ("+poster.Lang+") fetch OK")
					title := movie.Title[poster.Lang].Title
					posters = append(posters, posterData{image: image, lang: poster.Lang, title: title})
					break posterFetchLoop

				case themoviedb.ErrRateLimit:
					if i == (tmdbMaxRetries - 1) {
						journal.Error(crawlerID, " movie [", id, "] poster ("+poster.Lang+") fetch fail")
					} else {
						journal.Info(crawlerID, " tmdb rate limit exceeded, sleeping for "+rateLimitStr+" sec")
						time.Sleep(themoviedb.APIRateLimitDur)
					}

				default:
					journal.Error(crawlerID, " movie [", id, "] poster ("+poster.Lang+") fetch error: ", err)
					break posterLoop
				}
			}
		}
		if err != nil {
			_, err := movieFetchFailStmt.Exec(id)
			if err != nil {
				journal.Info(crawlerID, " ", err)
			}
			continue
		}

		// Добавляем полученные данные в БД.
		err = conn.Begin()
		if err != nil {
			journal.Error(crawlerID, " cannot begin transaction: ", err)
			continue
		}

		// Идентификатор фильма в БД, т.е. значение поля основного ключа в таблице movie.
		var movieDatabaseID int64

		// Добавляем фильм в БД.
		result, err := movieInsertStmt.Exec(
			movie.TMDBID,
			movie.OriginalTitle,
			movie.OriginalLang,
			movie.ReleaseDate.Format("2006-01-02"),
			movie.Adult,
			movie.IMDBID,
			movie.Popularity)
		if err != nil {
			goto DBError
		}

		// Получаем идентификатор, с которым фильм был добавлен в таблицу movie.
		movieDatabaseID, err = result.LastInsertId()
		if err != nil {
			goto DBError
		}
		// Если фильм уже есть в БД, то получаем его id вручную (PRIMARY KEY).
		if movieDatabaseID == 0 {
			err = movieIDStmt.QueryRow(id).Scan(&movieDatabaseID)
			if err != nil {
				goto DBError
			}
		}

		// Добавляем постеры в БД.
		for _, poster := range posters {
			_, err = posterInsertStmt.Exec(movieDatabaseID, poster.lang, poster.title, poster.image)
			if err != nil {
				goto DBError
			}
		}

		// Сохраняем успешность скачивания фильма.
		_, err = movieFetchSuccessStmt.Exec(id)
		if err != nil {
			goto DBError
		}

		// Если мы дошли до этого места, то значит все данные готовы к добавлению в БД.
		err = conn.Commit()
		if err != nil {
			journal.Error(crawlerID, " ", err)
		} else {
			journal.Info(crawlerID, " movie [", id, "] add to database OK")
		}
		continue mainLoop

	DBError:
		journal.Error(crawlerID, " ", err, ", rolling back")
		err = conn.Rollback()
		if err != nil {
			journal.Error(crawlerID, " ", err)
		}
	}
}

// Строка длины length из случайных букв верхнего регистра английского
// алфавита.
func genRandString(length int) string {
	if length <= 0 {
		return ""
	}

	randStr := ""
	alphabet := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	for i := 0; i < length; i++ {
		randStr = randStr + string(alphabet[rand.Intn(length)])
	}
	return randStr
}
