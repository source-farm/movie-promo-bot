package main

import (
	"bot/iso6391"
	"bot/journal"
	"bot/sqlite"
	"bot/themoviedb"
	"bufio"
	"compress/gzip"
	"context"
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
	minVoteCountDefault = 25
	// Мин. количество голосов, которое должно быть у фильма на русском,
	// чтобы для него закачался постер.
	minVoteCountRu     = 5
	crawlersNum        = 3
	movieFetchMaxFails = 3
	tmdbMaxRetries     = 3
	dbBusyTimeoutMS    = 10000
	httpReqTimeout     = time.Second * 15

	// UPSERT - специальное расширение SQLite. Подробнее по ссылке
	// https://sqlite.org/lang_UPSERT.html
	// Следующий запрос работает так: если строки с переданным значением
	// tmdb_id в таблице нет, то происходит вставка. Если есть, то выполняется
	// обновление полей с установкой поля updated_on в текущее время.
	movieUpsertQuery = `
INSERT INTO movie (tmdb_id,
                   original_title,
                   original_lang,
                   released_on,
                   adult,
                   imdb_id,
                   vote_count,
                   vote_average,
                   collection_id)
     VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
ON CONFLICT (tmdb_id) DO UPDATE SET (tmdb_id,
                                     original_title,
                                     original_lang,
                                     released_on,
                                     adult,
                                     imdb_id,
                                     vote_count,
                                     vote_average,
                                     collection_id,
                                     updated_on) = (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, datetime('now'));
`

	movieDBIDQuery = `
SELECT id
  FROM movie
 WHERE tmdb_id = ?1;
`

	// Языки, для которых есть постеры в БД для переданного идентификатора фильма.
	posterLangsQuery = `
    SELECT md.lang
      FROM movie as m
INNER JOIN movie_detail as md on m.id = md.fk_movie_id
     WHERE m.tmdb_id = ?1;
`

	posterInsertQuery = `
INSERT INTO movie_detail (fk_movie_id, lang, title, poster)
     VALUES (?1, ?2, ?3, ?4);
`
)

// Краткая информация о фильме из файла ежедневного экспорта The MovieDB API.
type movieBrief struct {
	TMDBID int `json:"id"` // Идентификатор фильма в The MovieDB API.
}

// theMovieDBHarvester заполняет локальную базу фильмов через The MovieDB API.
func theMovieDBHarvester(ctx context.Context, finished *sync.WaitGroup, key, dbName string) {
	journal.Replace(key, "<themoviedbapi_key>")
	goID := "[go tmdb-harvester]:"
	journal.Info(goID, " started")

	defer func() {
		finished.Done()
		journal.Info(goID, " finished")
	}()

	httpClient := &http.Client{
		Timeout: httpReqTimeout,
	}
	tmdbClient := themoviedb.NewClient(key, httpClient)
	err := tmdbClient.Configure()
	if err != nil {
		journal.Fatal(goID, " ", err)
	}

	// Для ожидания завершения tmdbSeeker'а и tmdbCrawler'ов.
	var wg sync.WaitGroup

	for {
		journal.Info(goID, " starting new movies fetch")
		wg.Add(crawlersNum + 1) // +1 для горутины tmdbSeeker.

		movieID := make(chan int)
		// tmdbSeeker записывает в канал movieID идентификаторы фильмов, для
		// которых ещё не скачаны все постеры.  Горутины tmdbCrawler извлекают
		// эти идентификаторы из movieID и выполняют фактическую работу по
		// скачиванию и добавлению фильмов в БД.
		go tmdbSeeker(ctx, &wg, tmdbClient, dbName, movieID)
		for i := 0; i < crawlersNum; i++ {
			crawlerID := "[go tmdb-crawler-" + strconv.Itoa(i+1) + "]:"
			go tmdbCrawler(crawlerID, &wg, tmdbClient, dbName, movieID)
		}

		wg.Wait()
		journal.Info(goID, " movies fetch finished")

		// После завершения сессии получения фильмов ждём начала следующего дня
		// по UTC перед следующей сессией.
		nextDay := time.Now().AddDate(0, 0, 1)
		nextDay = time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), 0, 0, 0, 0, time.UTC)
		sleepDuration := time.Until(nextDay)
		journal.Info(goID, "sleeping for ", sleepDuration, " (before ", nextDay, ")")
		timer := time.NewTimer(sleepDuration)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			journal.Info(goID, " sleeping cancelled")
			return
		}

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
func tmdbSeeker(ctx context.Context, wg *sync.WaitGroup, client *themoviedb.Client, dbName string, movieID chan<- int) {
	goID := "[go tmdb-seeker]:"
	dailyExportFilename := "daily"

	journal.Info(goID, " started")

	// Очистка по завершению.
	defer func() {
		if _, err := os.Stat(dailyExportFilename); err == nil {
			os.Remove(dailyExportFilename)
		}
		close(movieID)
		wg.Done()
		journal.Info(goID, " finished")
	}()

	// Установка соединения с БД и её настройка.
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

	// Подготовка запросов.
	posterLangsStmt, err := conn.Prepare(posterLangsQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer posterLangsStmt.Close()
	journal.Trace(goID, " poster languages query prepared")

	movieDBIDStmt, err := conn.Prepare(movieDBIDQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer movieDBIDStmt.Close()
	journal.Trace(goID, " movie id query prepared")

	//--------------------------------------------------------------------------------
	// Обрабатываем фильмы из базы с краткой информацией о всех фильмах The MovieDB API.
	//--------------------------------------------------------------------------------

	// Пытаемся скачать базу с краткой информацией о фильмах за какой-либо из
	// пяти предыдущих дней.
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
	// Извлекаем этот объект и отправляем фильм в канал movieID, если его нет в БД.
	scanner := bufio.NewScanner(gzipReader)
	for scanner.Scan() {
		movieRaw := scanner.Text()
		var movie movieBrief
		err = json.Unmarshal([]byte(movieRaw), &movie)
		if err != nil || movie.TMDBID == 0 {
			continue
		}

		var movieDBID int64
		err = movieDBIDStmt.QueryRow(movie.TMDBID).Scan(&movieDBID)
		if err != nil && err != sqlite.ErrNoRows {
			journal.Error(goID, " ", err)
			continue
		}

		// Отправляем фильм дальше, если его нет в БД.
		if err == sqlite.ErrNoRows {
			select {
			case movieID <- movie.TMDBID:
			case <-ctx.Done():
				return
			}
		} else {
			journal.Trace(goID, " ", "movie [", movie.TMDBID, "] is contained in DB, skip fetching")
		}
	}
	err = scanner.Err()
	if err != nil {
		journal.Error(goID, " ", err)
	}

	//--------------------------------------------------------------------------------
	// Обрабатываем изменившиеся фильмы.
	//--------------------------------------------------------------------------------
	journal.Info(goID, " processing changed movies")
	rateLimitStr := strconv.Itoa(int(themoviedb.APIRateLimitDur.Seconds()))
pagesLoop:
	for page := 1; page <= themoviedb.ChangedMoviesMaxPage; page++ {
		var movies []int
		var err error
	pageFetchLoop:
		for i := 0; i < tmdbMaxRetries; i++ {
			journal.Trace(goID, " fetching changed movies page #", page)
			movies, err = client.GetChangedMovies(page)
			switch err {
			case nil:
				journal.Info(goID, " changed movies page #", page, " fetch OK")
				break pageFetchLoop

			case themoviedb.ErrRateLimit:
				if i == (tmdbMaxRetries - 1) {
					journal.Info(goID, " changed movies page #", page, " fetch fail")
					break pageFetchLoop
				} else {
					journal.Info(goID, " tmdb rate limit exceeded, sleeping for "+rateLimitStr+" sec")
					time.Sleep(themoviedb.APIRateLimitDur)
				}

			case themoviedb.ErrPage:
				break pagesLoop

			default:
				journal.Error(goID, " changed movies page #", page, " fetch error: ", err)
				break pageFetchLoop
			}
		}

		for _, tmdbID := range movies {
			finished, err := allPostersFetched(posterLangsStmt, tmdbID)
			if err != nil {
				journal.Error(goID, " ", err)
				continue
			}

			if !finished {
				select {
				case movieID <- tmdbID:
				case <-ctx.Done():
					return
				}
			}
		}
	}
	journal.Info(goID, " changed movies processing end")
}

// tmdbCrawler извлекает по The MovieDB API данные о фильмах из movieID и
// записывает эти данные в БД.
func tmdbCrawler(goID string, wg *sync.WaitGroup, client *themoviedb.Client, dbName string, movieID <-chan int) {
	journal.Info(goID, " started")
	defer func() {
		wg.Done()
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

	// Подготовка запросов.
	movieUpsertStmt, err := conn.Prepare(movieUpsertQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer movieUpsertStmt.Close()
	journal.Trace(goID, " movie upsert query prepared")

	posterLangsStmt, err := conn.Prepare(posterLangsQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer posterLangsStmt.Close()
	journal.Trace(goID, " poster languages query prepared")

	posterInsertStmt, err := conn.Prepare(posterInsertQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer posterInsertStmt.Close()
	journal.Trace(goID, " poster insert query prepared")

	movieDBIDStmt, err := conn.Prepare(movieDBIDQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	defer movieDBIDStmt.Close()
	journal.Trace(goID, " movie id query prepared")

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
			continue
		}

		if movie.ReleaseDate.After(time.Now()) {
			journal.Info(goID, " movie [", tmdbID, "] has still not released, skip")
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
			// Находим для каких языков постеры уже есть в БД.
			inDBPosterLangs, err := getFetchedPosterLangs(posterLangsStmt, tmdbID)
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
				if _, ok := inDBPosterLangs[poster.Lang]; ok {
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

		// Объявляем movieDBID здесь, т.к. goto не может перепрыгивать через
		// объявления переменных.
		var movieDBID int64
		// Добавляем описание фильма в таблицу movie.
		journal.Trace(goID, " adding (or updading) movie [", tmdbID, "] description to database")
		_, err = movieUpsertStmt.Exec(
			movie.TMDBID,
			movie.OriginalTitle,
			movie.OriginalLang,
			movie.ReleaseDate.Format("2006-01-02"),
			movie.Adult,
			movie.IMDBID,
			movie.VoteCount,
			movie.VoteAverage,
			movie.Collection.ID)
		if err != nil {
			goto DBError
		}

		// Получаем идентификатор, с которым фильм был добавлен в таблицу movie.
		err = movieDBIDStmt.QueryRow(tmdbID).Scan(&movieDBID)
		if err != nil {
			goto DBError
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
			goto DBError
		} else {
			journal.Info(goID, " adding movie [", tmdbID, "] data to database OK")
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

// allPostersFetched возвращает true, nil есть все постеры фильма с
// идентификатором tmdbID уже получены.
func allPostersFetched(posterLangsStmt *sqlite.Stmt, tmdbID int) (bool, error) {
	inDBPosterLangs, err := getFetchedPosterLangs(posterLangsStmt, tmdbID)
	if err != nil {
		return false, err
	}
	_, EnPosterFetched := inDBPosterLangs[iso6391.En]
	_, RuPosterFetched := inDBPosterLangs[iso6391.Ru]
	return EnPosterFetched && RuPosterFetched, nil
}

// getFetchedPosterLangs находит языки, для которых постеры есть в БД.
func getFetchedPosterLangs(posterLangsStmt *sqlite.Stmt, tmdbID int) (map[iso6391.LangCode]struct{}, error) {
	rows, err := posterLangsStmt.Query(tmdbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posterLangs := map[iso6391.LangCode]struct{}{}
	for rows.Next() {
		var lang string
		err = rows.Scan(&lang)
		if err != nil {
			return nil, err
		}
		posterLangs[lang] = struct{}{}
	}
	if rows.Err() != nil {
		return nil, err
	}

	return posterLangs, nil
}
