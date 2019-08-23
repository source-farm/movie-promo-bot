// Пакет для выполнения запросов к сервису "https://api.themoviedb.org".
package themoviedb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
