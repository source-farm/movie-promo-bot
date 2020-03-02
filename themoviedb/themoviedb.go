// Пакет для выполнения запросов к сервису "https://api.themoviedb.org".
package themoviedb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/source-farm/movie-promo-bot/iso6391"
	"github.com/source-farm/movie-promo-bot/jsonstream"
)

const (
	// The MovieDB API устанавливает ограничение в 40 запросов за 10 секунд.
	apiRateLimit    = 40
	APIRateLimitDur = time.Millisecond * 10000

	// Файл с базой фильмов, который предоставляет The MovieDB API, не должен
	// превышать эту константу.
	movieDailyExportMaxSize = 50 * 1024 * 1024 // 50MiB

	// Максимальный номер страницы, которую можно запросить по пути
	// /movie/now_playing при обращении к The MovieDB API.
	NowPlayingMaxPage = 500

	// Максимальный номер страницы, которую можно запросить по пути
	// /movie/changes при обращении к The MovieDB API.
	ChangedMoviesMaxPage = 1000
)

// Переменные для контроля лимита запросов. Т.к. The MovieDB API устанавливает
// лимит запросов на основе IP адреса, создаём эти переменные на уровне пакета,
// т.е. они будут общими для всех Client'ов.
var (
	mu            sync.Mutex
	reqLimit      int       // Количество доступных запросов.
	reqLimitStart time.Time // Начало нового отсчёта лимита запросов.
)

type configuration struct {
	Images struct {
		BaseURL     string   `json:"base_url"`
		PosterSizes []string `json:"poster_sizes"`
	} `json:"images"`
	posterSize string
}

// Языки, которые поддерживаются Client'ом.
var supportedLangs = map[iso6391.LangCode]struct{}{
	iso6391.En: struct{}{},
	iso6391.Ru: struct{}{},
}

type translation struct {
	Lang iso6391.LangCode `json:"iso_639_1"`
	Data struct {
		Title string `json:"title"`
	} `json:"data"`
}

var (
	// Превышение лимита запросов к The MovieDB API.
	ErrRateLimit = errors.New("themoviedb: API rate limit exceeded")

	// Клиент не настроен.
	ErrConfigure = errors.New("themoviedb: client is not configured (call Configure method)")

	// Запрос несуществующей страницы при выполнении запросов, которые
	// возвращают результаты постранично. Например, метод GetNowPlaying структуры Client.
	ErrPage = errors.New("themoviedb: page not found")
)

// Client позволяет выполнять запросы к TheMovieDB API.
type Client struct {
	key        string
	httpClient *http.Client
	apiBaseURL string
	configMu   sync.Mutex
	config     configuration
}

// Poster хранит информацию о постере фильма.
type Poster struct {
	Lang        iso6391.LangCode `json:"iso_639_1"`
	Path        string           `json:"file_path"`
	VoteAverage float64          `json:"vote_average"`
}

// MovieCollection описывает коллекцию фильма - так группируются разные части
// одного фильма.
type MovieCollection struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Movie - это информация об одном фильме.
type Movie struct {
	TMDBID        int                         `json:"id"`
	IMDBID        string                      `json:"imdb_id"`
	OriginalTitle string                      `json:"original_title"`
	OriginalLang  iso6391.LangCode            `json:"original_language"`
	Adult         bool                        `json:"adult"`
	ReleaseDate   time.Time                   `json:"-"`
	VoteCount     int                         `json:"vote_count"`
	VoteAverage   float64                     `json:"vote_average"`
	Collection    MovieCollection             `json:"belongs_to_collection"`
	Title         map[iso6391.LangCode]string `json:"-"`
	Poster        map[iso6391.LangCode]Poster `json:"-"`
}

// NewClient возвращает новый TheMovieDB API клиент. Если httpClient равен
// nil, то возвращаемый клиент будет пользоваться http.DefaultClient'ом.
func NewClient(key string, httpClient *http.Client) *Client {
	client := Client{
		key:        key,
		httpClient: httpClient,
		apiBaseURL: "http://api.themoviedb.org/3",
	}
	if client.httpClient == nil {
		client.httpClient = http.DefaultClient
	}
	return &client
}

// Configure получает настройки TheMovieDB API (GET /configuration). Удачный
// вызов этого метода заполняет поле config клиента, который необходим при
// выполнении запросов для получения постеров. В документации к TheMovieDB API
// рекомендуют получать настройки раз в несколько дней.
// При возврате ошибки ErrRateLimit нужно ждать некоторое время перед
// выполнением следующим вызовом.
func (c *Client) Configure() error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	url, err := url.Parse(c.apiBaseURL + "/configuration")
	if err != nil {
		return err
	}
	query := url.Query()
	query.Add("api_key", c.key)
	url.RawQuery = query.Encode()

	err = c.checkRateLimit()
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Get(url.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("themoviedb: " + resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, &c.config)
	if err != nil {
		return err
	}

	// Сразу же подбираем нужный размер постера.
	w500Found := false
	w780Found := false
	for _, size := range c.config.Images.PosterSizes {
		switch size {
		case "w500":
			w500Found = true
		case "w780":
			w780Found = true
		}
	}
	switch {
	case w500Found:
		c.config.posterSize = "w500"
	case w780Found:
		c.config.posterSize = "w780"
	default:
		c.config.posterSize = "original"
	}

	return nil
}

// GetDailyExport позволяет скачать формируемый каждый день файл всех фильмов
// TheMovieDB. После 8:00 по UTC можно вычитывать новую базу фильмов. База
// представляет собой .gz файл, где каждая строка - это JSON с краткой
// информацией о фильме.
// Параметр filename - это путь к файлу, куда нужно сохранять базу фильмов.
// Для вызова этой функции клиент может не обладать ключом.
func (c *Client) GetDailyExport(year, month, day int, filename string) (err error) {
	// Формируем URL вида
	//
	// http://files.tmdb.org/p/exports/movie_ids_MM_DD_YEAR.json.gz"
	//
	date := fmt.Sprintf("%02d_%02d_%d", month, day, year)
	url := "http://files.tmdb.org/p/exports/movie_ids_" + date + ".json.gz"
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("themoviedb: " + resp.Status)
	}

	bodySize, err := strconv.Atoi(resp.Header.Get("Content-Length"))
	if err != nil {
		return err
	}
	if bodySize > movieDailyExportMaxSize {
		return errors.New("themoviedb: too large movie daily export")
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(filename)
		}
	}()
	readSize, err := io.Copy(file, resp.Body)
	if err != nil {
		return err
	}
	if readSize != int64(bodySize) {
		return errors.New("themoviedb: movie daily export size isn't equal to announced one")
	}

	return nil
}

// GetMovie возвращает информацию о фильме с идентификатором id в базе
// The MovieDB API. При возврате ошибки ErrRateLimit нужно ждать некоторое время
// перед выполнением следующего вызова.
func (c *Client) GetMovie(id int) (Movie, error) {
	// Формируем URL вида
	//
	// http://api.themoviedb.org/3/movie/<id>?api_key=<key>&append_to_response=translations,images
	//
	url, err := url.Parse(c.apiBaseURL + "/movie/" + strconv.Itoa(id))
	if err != nil {
		return Movie{}, err
	}
	query := url.Query()
	query.Add("api_key", c.key)
	query.Add("append_to_response", "translations,images")
	url.RawQuery = query.Encode()

	err = c.checkRateLimit()
	if err != nil {
		return Movie{}, err
	}

	resp, err := c.httpClient.Get(url.String())
	if err != nil {
		return Movie{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Movie{}, errors.New("themoviedb: " + resp.Status)
	}

	movie := Movie{}
	//- Настройка сканирования ответного JSON'а.
	scanner := jsonstream.NewScanner()
	scanner.SearchFor(&movie.TMDBID, "id")
	scanner.SearchFor(&movie.IMDBID, "imdb_id")
	scanner.SearchFor(&movie.OriginalTitle, "original_title")
	scanner.SearchFor(&movie.OriginalLang, "original_language")
	scanner.SearchFor(&movie.Adult, "adult")
	var releaseDateStr string
	scanner.SearchFor(&releaseDateStr, "release_date")
	scanner.SearchFor(&movie.VoteAverage, "vote_average")
	scanner.SearchFor(&movie.VoteCount, "vote_count")
	scanner.SearchFor(&movie.Collection, "belongs_to_collection")
	//- Названия фильма на различных языках.
	var translations []translation
	scanner.SearchFor(&translations, "translations", "translations")
	translationFilter := func(v interface{}) bool {
		t, ok := v.(translation)
		if !ok {
			return false
		}
		_, ok = supportedLangs[t.Lang]
		return ok
	}
	scanner.SetFilter(translationFilter, "translations", "translations")
	//- Постеры.
	var posters []Poster
	scanner.SearchFor(&posters, "images", "posters")
	posterFilter := func(v interface{}) bool {
		poster, ok := v.(Poster)
		if !ok {
			return false
		}
		_, ok = supportedLangs[poster.Lang]
		return ok
	}
	scanner.SetFilter(posterFilter, "images", "posters")

	//- Собственно само сканирование.
	err = scanner.Find(resp.Body)
	if err != nil {
		return Movie{}, err
	}

	// Извлекаем дату из строки.
	dateFormatISO := "2006-01-02"
	releaseDate, err := time.Parse(dateFormatISO, releaseDateStr)
	if err == nil {
		movie.ReleaseDate = releaseDate
	}

	// Отбираем названия фильмов на поддерживаемых пакетом языках.
	_, ok := supportedLangs[movie.OriginalLang]
	if len(translations) > 0 || ok {
		movie.Title = map[iso6391.LangCode]string{}
	}
	if ok {
		movie.Title[movie.OriginalLang] = movie.OriginalTitle
	}
	for i := range translations {
		lang := translations[i].Lang
		_, ok := supportedLangs[lang]
		if ok {
			_, ok := movie.Title[lang]
			if !ok {
				movie.Title[lang] = translations[i].Data.Title
			}
		}
	}

	// Отбираем самый популярный постер для каждого языка.
	if len(posters) > 0 {
		movie.Poster = map[iso6391.LangCode]Poster{}
	}
	for i := range posters {
		lang := posters[i].Lang
		_, ok := supportedLangs[lang]
		if !ok {
			continue
		}

		p, ok := movie.Poster[lang]
		if !ok || p.VoteAverage < posters[i].VoteAverage {
			movie.Poster[lang] = posters[i]
		}
	}

	return movie, nil
}

// GetNowPlaying находит фильмы, которые сейчас показывают в кинотеатрах.
// Фильмы разбиты по страницам, начиная с 1. Если страниц не осталось, то
// возвращается ошибка ErrPage.
func (c *Client) GetNowPlaying(page int) ([]Movie, error) {
	// Формируем URL вида
	//
	// http://api.themoviedb.org/3/movie/now_playing?api_key=<key>&page=<pageNum>
	//
	url, err := url.Parse(c.apiBaseURL + "/movie/now_playing")
	if err != nil {
		return nil, err
	}
	query := url.Query()
	query.Add("api_key", c.key)
	query.Add("page", strconv.Itoa(page))
	url.RawQuery = query.Encode()

	err = c.checkRateLimit()
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("themoviedb: " + resp.Status)
	}

	movies := []Movie{}
	scanner := jsonstream.NewScanner()
	scanner.SearchFor(&movies, "results")
	totalPages := 0
	scanner.SearchFor(&totalPages, "total_pages")
	err = scanner.Find(resp.Body)
	if err != nil {
		return nil, err
	}
	if page > totalPages {
		return nil, ErrPage
	}

	return movies, nil
}

// GetChangedMovies возвращает идентификаторы фильмов (TMDBID), которые были
// изменены за предыдущий день.
// Фильмы разбиты по страницам, начиная с 1. Если страниц не осталось, то
// возвращается ошибка ErrPage.
func (c *Client) GetChangedMovies(page int) ([]int, error) {
	// Формируем URL вида
	//
	// http://api.themoviedb.org/3/movie/changes?api_key=<key>&end_date=<end_date>&start_date=<start_date>&page=<pageNum>
	//
	url, err := url.Parse(c.apiBaseURL + "/movie/changes")
	if err != nil {
		return nil, err
	}
	query := url.Query()
	query.Add("api_key", c.key)
	query.Add("page", strconv.Itoa(page))
	now := time.Now()
	query.Add("end_date", now.Format("2006-01-02"))
	prevDay := now.AddDate(0, 0, -1)
	query.Add("start_date", prevDay.Format("2006-01-02"))
	url.RawQuery = query.Encode()

	err = c.checkRateLimit()
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("themoviedb: " + resp.Status)
	}

	changedMovies := []Movie{}
	scanner := jsonstream.NewScanner()
	scanner.SearchFor(&changedMovies, "results")
	totalPages := 0
	scanner.SearchFor(&totalPages, "total_pages")
	err = scanner.Find(resp.Body)
	if err != nil {
		return nil, err
	}
	if page > totalPages {
		return nil, ErrPage
	}

	changedMovieIDs := make([]int, len(changedMovies))
	for i := range changedMovies {
		changedMovieIDs[i] = changedMovies[i].TMDBID
	}

	return changedMovieIDs, nil
}

// GetPoster закачивает постер через путь к нему.
// При возврате ошибки ErrRateLimit нужно ждать некоторое время перед
// выполнением следующим вызовом.
func (c *Client) GetPoster(path string) ([]byte, error) {
	// Формируем URL вида
	//
	// http://image.tmdb.org/t/p/<size>/<poster>
	//
	c.configMu.Lock()
	if c.config.Images.BaseURL == "" {
		c.configMu.Unlock()
		return nil, ErrConfigure
	}
	url, err := url.Parse(c.config.Images.BaseURL + "/" + c.config.posterSize + path)
	c.configMu.Unlock()
	if err != nil {
		return nil, err
	}

	err = c.checkRateLimit()
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("themoviedb: " + resp.Status)
	}
	poster, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Простая проверка, что полученные данные являются JPEG картинкой.
	_, err = jpeg.DecodeConfig(bytes.NewReader(poster))
	if err != nil {
		// Если данные не являются JPEG картинкой, то проверяем являются ли они PNG.
		_, err = png.DecodeConfig(bytes.NewReader(poster))
		if err != nil {
			return nil, err
		}
	}

	return poster, nil
}

func (c *Client) checkRateLimit() error {
	mu.Lock()
	defer mu.Unlock()
	if reqLimitStart.IsZero() || time.Since(reqLimitStart) > APIRateLimitDur {
		reqLimit = apiRateLimit
		reqLimitStart = time.Now()
	}
	if reqLimit == 0 {
		return ErrRateLimit
	}
	reqLimit--
	return nil
}
