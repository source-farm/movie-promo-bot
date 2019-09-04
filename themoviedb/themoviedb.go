// Пакет для выполнения запросов к сервису "https://api.themoviedb.org".
package themoviedb

import (
	"bot/iso6391"
	"bot/jsonstream"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

var movieDailyExportMaxSize = 50 * 1024 * 1024 // 50MiB

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

// Client позволяет выполнять запросы к TheMovieDB API.
type Client struct {
	key            string
	httpClient     *http.Client
	apiBaseURL     string
	config         configuration
	lastConfigured time.Time
}

// Title хранит название фильма.
type Title struct {
	Lang  iso6391.LangCode `json:"iso_639_1"`
	Title string           `json:"title"`
}

// Poster хранит информацию о постере фильма.
type Poster struct {
	Lang        iso6391.LangCode `json:"iso_639_1"`
	Path        string           `json:"file_path"`
	VoteAverage float64          `json:"vote_average"`
}

// Movie представляет собой информацию об одном фильме.
type Movie struct {
	TMDBID        int
	IMDBID        string
	OriginalTitle string
	OriginalLang  iso6391.LangCode
	Adult         bool
	ReleaseDate   time.Time
	Popularity    float64
	Title         map[iso6391.LangCode]Title
	Poster        map[iso6391.LangCode]Poster
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
// выполнении запросов для получения изображений. В документации к
// TheMovieDB API рекомендуют получать настройки раз в несколько дней.
func (c *Client) Configure() error {
	url, err := url.Parse(c.apiBaseURL + "/configuration")
	if err != nil {
		return err
	}
	query := url.Query()
	query.Add("api_key", c.key)
	url.RawQuery = query.Encode()

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

// GetMovie возвращает информацию о фильме с идентификатором id.
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
	scanner.SearchFor(&movie.Popularity, "popularity")
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
	err = scanner.Find(resp.Body)
	if err != nil {
		return Movie{}, err
	}

	dateFormatISO := "2006-01-02"
	releaseDate, err := time.Parse(dateFormatISO, releaseDateStr)
	if err == nil {
		movie.ReleaseDate = releaseDate
	}

	_, ok := supportedLangs[movie.OriginalLang]
	if len(translations) > 0 || ok {
		movie.Title = map[iso6391.LangCode]Title{}
	}
	if ok {
		movie.Title[movie.OriginalLang] = Title{Lang: movie.OriginalLang, Title: movie.OriginalTitle}
	}
	for i := range translations {
		lang := translations[i].Lang
		_, ok := supportedLangs[lang]
		if ok {
			_, ok := movie.Title[lang]
			if !ok {
				movie.Title[lang] = Title{Lang: lang, Title: translations[i].Data.Title}
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

// GetPoster закачивает постер через путь к нему.
func (c *Client) GetPoster(path string) ([]byte, error) {
	if time.Since(c.lastConfigured) > time.Second*3600*24 {
		err := c.Configure()
		if err != nil {
			return nil, err
		}
		c.lastConfigured = time.Now()
	}

	// Формируем URL вида
	//
	// http://image.tmdb.org/t/p/<size>/<poster>
	//
	url, err := url.Parse(c.config.Images.BaseURL + "/" + c.config.posterSize + path)
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

	return poster, nil
}
