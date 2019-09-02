// Пакет для выполнения запросов к сервису "https://api.themoviedb.org".
package themoviedb

import (
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
}

// Client позволяет выполнять запросы к TheMovieDB API.
type Client struct {
	key        string
	httpClient *http.Client
	apiBaseURL string
	config     configuration
}

// Title хранит название фильма.
type Title struct {
	CountryISO31661 string `json:"iso_3166_1"`
	Title           string `json:"title"`
}

// Poster хранит информацию о постере фильма.
type Poster struct {
	LangISO6391 string `json:"iso_639_1"`
	Path        string `json:"file_path"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

// Movie представляет собой информацию об одном фильме.
type Movie struct {
	TMDBID              int
	IMDBID              string
	Adult               bool
	ReleaseDate         time.Time
	Popularity          float64
	OriginalLangISO6391 string
	Titles              []Title
	Posters             []Poster
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
	// http://api.themoviedb.org/3/movie/<id>?api_key=<key>&append_to_response=alternative_titles,images
	//
	url, err := url.Parse(c.apiBaseURL + "/movie/" + strconv.Itoa(id))
	if err != nil {
		return Movie{}, err
	}
	query := url.Query()
	query.Add("api_key", c.key)
	query.Add("append_to_response", "alternative_titles,images")
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
	scanner := jsonstream.NewScanner()
	scanner.SearchFor(&movie.TMDBID, "id")
	scanner.SearchFor(&movie.IMDBID, "imdb_id")
	scanner.SearchFor(&movie.Adult, "adult")
	var releaseDateStr string
	scanner.SearchFor(&releaseDateStr, "release_date")
	scanner.SearchFor(&movie.Popularity, "popularity")
	scanner.SearchFor(&movie.OriginalLangISO6391, "original_language")
	scanner.SearchFor(&movie.Titles, "alternative_titles", "titles")
	titleFilter := func(v interface{}) bool {
		title, ok := v.(Title)
		if !ok {
			return false
		}
		return title.CountryISO31661 == "US" || title.CountryISO31661 == "RU"
	}
	scanner.SetFilter(titleFilter, "alternative_titles", "titles")
	scanner.SearchFor(&movie.Posters, "images", "posters")
	posterFilter := func(v interface{}) bool {
		poster, ok := v.(Poster)
		if !ok {
			return false
		}
		return poster.LangISO6391 == "en" || poster.LangISO6391 == "ru"
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

	return movie, nil
}
