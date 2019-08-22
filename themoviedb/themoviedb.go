// Пакет для выполнения запросов к сервису "https://api.themoviedb.org".
package themoviedb

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
)

var movieDailyExportMaxSize = 50 * 1024 * 1024 // 50MiB

// Client позволяет выполнять запросы к TheMovieDB API.
type Client struct {
	key        string
	httpClient *http.Client
}

// NewClient возвращает новый TheMovieDB API клиент. Если httpClient равен
// nil, то возвращаемый клиент будет пользоваться http.DefaultClient'ом.
func NewClient(key string, httpClient *http.Client) *Client {
	client := Client{
		key:        key,
		httpClient: httpClient,
	}
	if client.httpClient == nil {
		client.httpClient = http.DefaultClient
	}
	return &client
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
