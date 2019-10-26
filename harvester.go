package main

import (
	"bot/iso6391"
	"bot/journal"
	"bot/sqlite"
	"bot/themoviedb"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	// Мин. количество голосов по-умолчанию, которое должно быть у фильма, чтобы
	// для него закачался постер.
	minVoteCountDefault = 100
	// Мин. количество голосов, которое должно быть у фильма на русском,
	// чтобы для него закачался постер.
	minVoteCountRu     = 10
	crawlersNum        = 3
	movieFetchMaxFails = 3
	tmdbMaxRetries     = 3
	dbBusyTimeoutMS    = 10000
	httpReqTimeout     = time.Second * 15

	movieInsertQuery = `
INSERT INTO movie (tmdb_id, original_title, original_lang, released_on, adult, imdb_id, vote_count, vote_average)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
ON CONFLICT (tmdb_id) DO NOTHING;
`

	movieIDQuery = `
SELECT id
  FROM movie
 WHERE tmdb_id = ?1;
`

	posterLangQuery = `
SELECT lang
  FROM movie_detail
 WHERE fk_movie_id = ?1;
`

	posterInsertQuery = `
INSERT INTO movie_detail (fk_movie_id, lang, title, poster)
VALUES (?1, ?2, ?3, ?4);
`

	movieFetchCompleteQuery = `
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
	TMDBID int `json:"id"` // Идентификатор фильма в The MovieDB API.
}

// Инициализация БД фильмов.
// TODO: перенести в main.
func initDB(goID, dbName string) {
	journal.Info(goID, " initialising database "+dbName)

	con, err := sqlite.NewConn(dbName)
	if err != nil {
		journal.Fatal(err)
	}
	defer con.Close()
	journal.Trace(goID, " connected to "+dbName)

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
    created_on     TEXT DEFAULT (datetime('now')),
    updated_on     TEXT
);
`
	_, err = con.Exec(query)
	if err != nil {
		journal.Fatal(goID, " ", err)
	}
	journal.Trace(goID, " table movie create OK")

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
		journal.Fatal(goID, " ", err)
	}
	journal.Trace(goID, " table movie_detail create OK")

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
		journal.Fatal(goID, " ", err)
	}
	journal.Trace(goID, " table movie_fetch create OK")

	journal.Info(goID, " database "+dbName+" init OK")
}

// theMovieDBHarvester заполняет локальную базу фильмов через The MovieDB API.
func theMovieDBHarvester(key, dbName string) {
	goID := "[go tmdb-harvester]:"
	journal.Info(goID, " started")

	initDB(goID, dbName)

	httpClient := &http.Client{
		Timeout: httpReqTimeout,
	}
	tmdbClient := themoviedb.NewClient(key, httpClient)
	err := tmdbClient.Configure()
	if err != nil {
		journal.Fatal(goID, " ", err)
	}

	// Для ожидания завершения tmdbCrawler'ов.
	var wg sync.WaitGroup

	for {
		journal.Info(goID, " starting new movies fetch")
		wg.Add(crawlersNum)

		movieID := make(chan int)
		// moviesSeeker записывает в канал movieID идентификаторы ещё не
		// полностью скачанных фильмов. Горутины tmdbCrawler извлекают эти
		// идентификаторы из movieID и выполняют фактическую работу по
		// скачиванию и добавлению фильмов в БД.
		go tmdbSeeker(tmdbClient, dbName, movieID)
		for i := 0; i < crawlersNum; i++ {
			crawlerID := "[go tmdb-crawler-" + strconv.Itoa(i+1) + "]:"
			go tmdbCrawler(crawlerID, &wg, tmdbClient, dbName, movieID)
		}

		wg.Wait()
		journal.Info(goID, " movies fetch finished")

		// После завершения сессии получения фильмов ждём 1 день перед следующей сессией.
		journal.Info(goID, " sleeping for 1 day")
		time.Sleep(time.Hour * 24)

		// Пытаемся снова сконфигурировать The Movie DB API клиента, т.к.
		// документация рекомендует это делать раз в несколько дней.
		err := tmdbClient.Configure()
		if err != nil {
			journal.Error(goID, " ", err)
		}
	}
}

// tmdbSeeker записывает в канал movieID идентификаторы фильмов, для которых ещё
// не была найдена вся необходимая информация.
func tmdbSeeker(client *themoviedb.Client, dbName string, movieID chan<- int) {
	goID := "[go tmdb-seeker]:"
	dailyExportFilename := "daily"

	journal.Info(goID, " started")

	// Очистка по завершению.
	defer func() {
		if _, err := os.Stat(dailyExportFilename); err == nil {
			os.Remove(dailyExportFilename)
		}
		close(movieID)
		journal.Info(goID, " finished")
	}()

	// Установка связи с БД, подготовка запросов.
	conn, err := sqlite.NewConn(dbName)
	if err != nil {
		journal.Error(goID, " ", err)
		return
	}
	defer conn.Close()
	err = conn.SetBusyTimeout(dbBusyTimeoutMS)
	if err != nil {
		journal.Error(goID, " ", err)
		return
	}
	fetchResultStmt, err := conn.Prepare(movieFetchResultQuery)
	if err != nil {
		journal.Error(goID, " ", err)
		return
	}
	defer fetchResultStmt.Close()

	// Обрабатываем фильмы, которые сейчас идут в кинотеатрах.
	journal.Info(goID, " processing now playing movies")
	rateLimitStr := strconv.Itoa(int(themoviedb.APIRateLimitDur.Seconds()))
pagesLoop:
	for page := 1; page <= themoviedb.NowPlayingMaxPage; page++ {
		var movies []themoviedb.Movie
		var err error
	pageFetchLoop:
		for i := 0; i < tmdbMaxRetries; i++ {
			journal.Trace(goID, " fetching now playing page #", page)
			movies, err = client.GetNowPlaying(page)
			switch err {
			case nil:
				journal.Info(goID, " now playing page #", page, " fetch OK")
				break pageFetchLoop

			case themoviedb.ErrRateLimit:
				if i == (tmdbMaxRetries - 1) {
					journal.Info(goID, " now playing page #", page, " fetch fail")
					break pageFetchLoop
				} else {
					journal.Info(goID, " tmdb rate limit exceeded, sleeping for "+rateLimitStr+" sec")
					time.Sleep(themoviedb.APIRateLimitDur)
				}

			case themoviedb.ErrPage:
				break pagesLoop

			default:
				journal.Error(goID, " now playing page #", page, " fetch error: ", err)
				break pageFetchLoop
			}
		}

		for _, movie := range movies {
			finished, err := movieFetchFinished(fetchResultStmt, movie.TMDBID, goID)
			if err != nil {
				journal.Error(goID, " ", err)
				continue
			}

			if !finished {
				movieID <- movie.TMDBID
			}
		}
	}
	journal.Info(goID, " now playing movies processing end")

	// Обрабатываем фильмы из базы с краткой информацией о всех фильмах
	// The MovieDB API. Пытаемся скачать эту базу за какой-либо из пяти
	// предыдущих дней.
	now := time.Now()
	for i := 1; i <= 5; i++ {
		date := now.AddDate(0, 0, -i)
		journal.Info(goID, " downloading daily export for "+date.Format("2006-01-02"))
		year, month, day := date.Date()
		err = client.GetDailyExport(year, int(month), day, dailyExportFilename)
		if err == nil {
			journal.Info(goID, " daily export for "+date.Format("2006-01-02")+" download OK")
			break
		} else {
			journal.Error(goID, " daily export for "+date.Format("2006-01-02")+" download fail: ", err)
		}
	}
	if err != nil {
		journal.Error(goID, " cannot download daily export for any of 5 previous days")
		return
	}

	// Файл базы фильмов - это архив gzip. Извлекаем из него данные на лету.
	f, err := os.Open(dailyExportFilename)
	if err != nil {
		journal.Error(goID, " ", err)
		return
	}
	defer f.Close()
	gzipReader, err := gzip.NewReader(f)
	if err != nil {
		journal.Error(goID, " ", err)
		return
	}
	defer gzipReader.Close()

	// Каждая строка в базе фильмов - это JSON объект с краткой информацией о фильме.
	// Извлекаем этот объект и отправляем фильм в канал movieID, если нужно.
	scanner := bufio.NewScanner(gzipReader)
	for scanner.Scan() {
		movieRaw := scanner.Text()
		var movie movieBrief
		err = json.Unmarshal([]byte(movieRaw), &movie)
		if err != nil || movie.TMDBID == 0 {
			continue
		}

		finished, err := movieFetchFinished(fetchResultStmt, movie.TMDBID, goID)
		if err != nil {
			journal.Error(goID, " ", err)
			continue
		}

		if !finished {
			movieID <- movie.TMDBID
		}
	}
	err = scanner.Err()
	if err != nil {
		journal.Error(goID, " ", err)
	}
}

// tmdbCrawler извлекает по The MovieDB API данные о фильмах из movieID и
// записывает эти данные в БД.
func tmdbCrawler(goID string, wg *sync.WaitGroup, client *themoviedb.Client, dbName string, movieID <-chan int) {
	defer wg.Done()

	journal.Info(goID, " started")
	defer func() {
		journal.Info(goID, " finished")
	}()

	// Установка соединения с БД и её настройка.
	conn, err := sqlite.NewConn(dbName)
	if err != nil {
		journal.Error(err)
		return
	}
	defer conn.Close()
	journal.Info(goID, " connected to database "+dbName)

	err = conn.SetBusyTimeout(dbBusyTimeoutMS)
	if err != nil {
		journal.Error(goID, " ", err)
		return
	}
	journal.Trace(goID, " set database connection busy timeout to ", dbBusyTimeoutMS, " ms")

	// Подготавливаем запросы.
	movieInsertStmt, err := conn.Prepare(movieInsertQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer movieInsertStmt.Close()
	journal.Trace(goID, " movie insert query prepared")

	movieIDStmt, err := conn.Prepare(movieIDQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer movieIDStmt.Close()
	journal.Trace(goID, " movie id query prepared")

	posterLangStmt, err := conn.Prepare(posterLangQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer posterLangStmt.Close()
	journal.Trace(goID, " poster language query prepared")

	posterInsertStmt, err := conn.Prepare(posterInsertQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer posterInsertStmt.Close()
	journal.Trace(goID, " poster insert query prepared")

	movieFetchFailStmt, err := conn.Prepare(movieFetchFailQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer movieFetchFailStmt.Close()
	journal.Trace(goID, " movie fetch fail query prepared")

	movieFetchCompleteStmt, err := conn.Prepare(movieFetchCompleteQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer movieFetchCompleteStmt.Close()
	journal.Trace(goID, " movie fetch success query prepared")

	rateLimitStr := strconv.Itoa(int(themoviedb.APIRateLimitDur.Seconds()))
	// Закачиваем фильмы.
mainLoop:
	for tmdbID := range movieID {
		var movie themoviedb.Movie
		var err error
		// Получаем общие данные фильма.
	movieFetchLoop:
		for i := 0; i < tmdbMaxRetries; i++ {
			journal.Trace(goID, " fetching movie [", tmdbID, "]")
			movie, err = client.GetMovie(tmdbID)
			switch err {
			case nil:
				journal.Info(goID, " movie [", tmdbID, "] fetch OK")
				break movieFetchLoop

			case themoviedb.ErrRateLimit:
				if i == (tmdbMaxRetries - 1) {
					journal.Error(goID, " movie [", tmdbID, "] fetch fail")
				} else {
					journal.Info(goID, " tmdb rate limit exceeded, sleeping for "+rateLimitStr+" sec")
					time.Sleep(themoviedb.APIRateLimitDur)
				}

			default:
				journal.Error(goID, " movie [", tmdbID, "] fetch error: ", err)
				break movieFetchLoop
			}
		}
		if err != nil {
			_, err = movieFetchFailStmt.Exec(tmdbID)
			if err != nil {
				journal.Error(goID, " ", err)
			}
			continue
		}

		if movie.ReleaseDate.After(time.Now()) {
			journal.Info(goID, " movie [", tmdbID, "] has still not released, skip")
			continue
		}

		// Идентификатор фильма в БД, т.е. значение поля основного ключа в таблице movie.
		var movieDBID int64
		err = movieIDStmt.QueryRow(tmdbID).Scan(&movieDBID)
		if err != nil && err != sqlite.ErrNoRows {
			journal.Error(goID, " ", err)
			continue
		}

		type posterData struct {
			image []byte
			lang  iso6391.LangCode
			title string
		}
		var posters []posterData

		movieHighRanked := false
		if movie.OriginalLang == iso6391.Ru {
			movieHighRanked = movie.VoteCount >= minVoteCountRu
		} else {
			movieHighRanked = movie.VoteCount >= minVoteCountDefault
		}
		// Закачиваем постеры фильма, если фильм популярен.
		if movieHighRanked {
			// Находим какие постеры текущего фильма уже есть в БД.
			inDBPosters, err := findInDBPosters(posterLangStmt, movieDBID)
			if err != nil {
				journal.Error(goID, " ", err)
				continue
			}

			err = nil
		posterLoop:
			for _, poster := range movie.Poster {
				// Если нет названия фильма на том же языке, что и постер, то не скачиваем постер.
				if title, ok := movie.Title[poster.Lang]; !ok || title == "" {
					journal.Trace(goID, " movie [", tmdbID, "] has no title for poster ("+poster.Lang+"), skip fetching it")
					continue
				}
				// Не скачиваем постер, если он уже есть в БД.
				if _, ok := inDBPosters[poster.Lang]; ok {
					journal.Trace(goID, " movie [", tmdbID, "] poster ("+poster.Lang+") is already in database, skip fetching it")
					continue
				}
				var image []byte

			posterFetchLoop:
				for i := 0; i < tmdbMaxRetries; i++ {
					journal.Trace(goID, " fetching movie [", tmdbID, "] poster ("+poster.Lang+")")
					image, err = client.GetPoster(poster.Path)
					switch err {
					case nil:
						journal.Info(goID, " movie [", tmdbID, "] poster ("+poster.Lang+") fetch OK")
						title := movie.Title[poster.Lang]
						posters = append(posters, posterData{image: image, lang: poster.Lang, title: title})
						break posterFetchLoop

					case themoviedb.ErrRateLimit:
						if i == (tmdbMaxRetries - 1) {
							journal.Error(goID, " movie [", tmdbID, "] poster ("+poster.Lang+") fetch fail")
						} else {
							journal.Info(goID, " tmdb rate limit exceeded, sleeping for "+rateLimitStr+" sec")
							time.Sleep(themoviedb.APIRateLimitDur)
						}

					default:
						journal.Error(goID, " movie [", tmdbID, "] poster ("+poster.Lang+") fetch error: ", err)
						break posterLoop
					}
				}
			}
			if err != nil {
				_, err := movieFetchFailStmt.Exec(tmdbID)
				if err != nil {
					journal.Info(goID, " ", err)
				}
				continue
			}
		} else {
			journal.Trace(goID, " movie [", tmdbID, "] is low voted, skip posters fetching")
		}

		// Добавляем полученные данные в БД.
		err = conn.Begin()
		if err != nil {
			journal.Error(goID, " cannot begin transaction: ", err)
			continue
		}

		// Добавляем фильм в таблицу movie, если его там нет.
		movieInDatabase := movieDBID != 0
		if !movieInDatabase {
			journal.Trace(goID, " adding movie [", tmdbID, "] description to database")
			result, err := movieInsertStmt.Exec(
				movie.TMDBID,
				movie.OriginalTitle,
				movie.OriginalLang,
				movie.ReleaseDate.Format("2006-01-02"),
				movie.Adult,
				movie.IMDBID,
				movie.VoteCount,
				movie.VoteAverage)
			if err != nil {
				goto DBError
			}

			// Получаем идентификатор, с которым фильм был добавлен в таблицу movie.
			movieDBID, err = result.LastInsertId()
			if err != nil {
				goto DBError
			}
		} else {
			journal.Trace(goID, " movie [", tmdbID, "] description is already in database, skip adding it to database")
		}

		// Добавляем постеры в БД.
		for _, poster := range posters {
			journal.Trace(goID, " adding movie [", tmdbID, "] poster ("+poster.lang+") to database")
			_, err = posterInsertStmt.Exec(movieDBID, poster.lang, poster.title, poster.image)
			if err != nil {
				goto DBError
			}
		}

		// Если мы дошли до этого места, то значит все данные готовы к добавлению в БД.
		err = conn.Commit()
		if err != nil {
			journal.Error(goID, " ", err)
		} else {
			inDBPosters, err := findInDBPosters(posterLangStmt, movieDBID)
			if err != nil {
				journal.Error(goID, " ", err)
				continue
			}
			_, okEn := inDBPosters[iso6391.En]
			_, okRu := inDBPosters[iso6391.Ru]
			movieComplete := okEn && okRu // Все данные о фильме получены.

			// Фиксируем, что работа с фильмом завершена.
			// Для низкорейтингового фильма ждём примерно месяц с момента его
			// релиза пока он не станет высокорейтинговым.
			if movieComplete || !movieHighRanked && time.Since(movie.ReleaseDate) > time.Hour*24*30 {
				_, err = movieFetchCompleteStmt.Exec(tmdbID)
				if err != nil {
					journal.Error(goID, " ", err)
					continue
				}
			}

			if movieInDatabase && len(posters) == 0 {
				journal.Info(goID, " movie [", tmdbID, "] has nothing new to add to database")
			} else {
				journal.Info(goID, " adding movie [", tmdbID, "] data to database OK")
			}
		}
		continue mainLoop

	DBError:
		journal.Error(goID, " ", err, ", rolling back")
		err = conn.Rollback()
		if err != nil {
			journal.Error(goID, " ", err)
		}
	}
}

// movieFetchFinished выясняет закончена ли работа с фильмом.
func movieFetchFinished(movieFetchResultStmt *sqlite.Stmt, tmdbID int, goID string) (bool, error) {
	var complete, failsNum int64
dbBusyLoop:
	for {
		err := movieFetchResultStmt.QueryRow(tmdbID).Scan(&complete, &failsNum)
		switch err {
		case nil:
			fallthrough

		case sqlite.ErrNoRows:
			break dbBusyLoop

		case sqlite.ErrBusy:
			fallthrough

		case sqlite.ErrLocked:
			journal.Error(goID, " ", err, ", sleeping for 1 sec")
			time.Sleep(time.Second)

		default:
			return false, err
		}
	}

	if complete == 1 {
		journal.Trace(goID, " movie [", tmdbID, "] fetch is complete, skip")
		return true, nil
	} else if failsNum >= movieFetchMaxFails {
		journal.Info(goID, " movie [", tmdbID, "] has too many fetch fails, skip")
		return true, nil
	}

	return false, nil
}

// findInDBPosters находит языки, для которых постеры есть в БД.
func findInDBPosters(stmt *sqlite.Stmt, movieDBID int64) (map[iso6391.LangCode]struct{}, error) {
	posters := map[iso6391.LangCode]struct{}{}

	rows, err := stmt.Query(movieDBID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var lang string
		err = rows.Scan(&lang)
		if err != nil {
			return nil, err
		}
		posters[lang] = struct{}{}
	}
	if rows.Err() != nil {
		return nil, err
	}

	return posters, nil
}
