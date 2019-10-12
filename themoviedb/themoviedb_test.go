package themoviedb

import (
	"bot/iso6391"
	"bytes"
	"errors"
	"image/jpeg"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const testMovieID = 550 // Fight Club
const testNowPlayingPage = 1

func TestDailyExport(t *testing.T) {
	filename := "daily_export.json.gz"
	client := NewClient("", nil)
	err := client.GetDailyExport(2019, 8, 1, filename)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filename)
}

func TestConfigure(t *testing.T) {
	key, err := getKey()
	if err != nil {
		t.Fatal(err)
	}

	client := NewClient(key, nil)
	err = client.Configure()
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetMovie(t *testing.T) {
	key, err := getKey()
	if err != nil {
		t.Fatal(err)
	}

	client := NewClient(key, nil)
	movie, err := client.GetMovie(testMovieID)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := movie.Title[iso6391.En]
	if !ok {
		t.Fatal("English title not found")
	}
}

func TestGetPoster(t *testing.T) {
	key, err := getKey()
	if err != nil {
		t.Fatal(err)
	}

	client := NewClient(key, nil)
	err = client.Configure()
	if err != nil {
		t.Fatal(err)
	}
	movie, err := client.GetMovie(testMovieID)
	if err != nil {
		t.Fatal(err)
	}

	poster, ok := movie.Poster[iso6391.En]
	if !ok {
		t.Fatal("No english poster")
	}
	data, err := client.GetPoster(poster.Path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("Cannot decode poster")
	}
}

func TestGetNowPlaying(t *testing.T) {
	key, err := getKey()
	if err != nil {
		t.Fatal(err)
	}

	// В результате выполнения предыдущего теста может не остаться запросов.
	time.Sleep(APIRateLimitDur)

	client := NewClient(key, nil)
	_, err = client.GetNowPlaying(499)
	if err != ErrPage {
		t.Fatalf("expected ErrPage, got %v", err)
	}

	for i := 0; i < apiRateLimit-1; i++ {
		_, err := client.GetNowPlaying(i + 1)
		if err != nil && err != ErrPage {
			t.Fatal(err)
		}
	}
}

func TestGetMovieConcurrent(t *testing.T) {
	key, err := getKey()
	if err != nil {
		t.Fatal(err)
	}

	// В результате выполнения предыдущего теста может не остаться запросов.
	time.Sleep(APIRateLimitDur)

	client := NewClient(key, nil)
	start := make(chan struct{})
	result := make(chan error)
	var lineUp sync.WaitGroup
	lineUp.Add(apiRateLimit)
	for i := 0; i < apiRateLimit; i++ {
		go func() {
			lineUp.Done()
			<-start
			_, err := client.GetMovie(testMovieID)
			result <- err
		}()
	}
	lineUp.Wait()
	close(start)
	reqFinished := 0
	timer := time.NewTimer(time.Second * 5)
loop:
	for {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
			reqFinished++
			if reqFinished == apiRateLimit {
				break loop
			}
		case <-timer.C:
			t.Fatal("Timeout")
		}
	}
}

func TestGetPosterConcurrent(t *testing.T) {
	key, err := getKey()
	if err != nil {
		t.Fatal(err)
	}

	// В результате выполнения предыдущего теста может не остаться запросов.
	time.Sleep(APIRateLimitDur)

	client := NewClient(key, nil)
	err = client.Configure()
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	result := make(chan error)
	var lineUp sync.WaitGroup
	postersNum := apiRateLimit/2 - 1 // client.Configure() забирает один запрос.
	lineUp.Add(postersNum)
	for i := 0; i < postersNum; i++ {
		go func() {
			lineUp.Done()
			<-start
			movie, _ := client.GetMovie(testMovieID)
			if poster, ok := movie.Poster[iso6391.En]; ok {
				data, err := client.GetPoster(poster.Path)
				if err != nil {
					result <- err
				} else {
					_, err = jpeg.Decode(bytes.NewReader(data))
					result <- err
				}
			} else {
				result <- errors.New("No english poster")
			}
		}()
	}
	lineUp.Wait()
	close(start)
	reqFinished := 0
	timer := time.NewTimer(time.Second * 10)
loop:
	for {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
			reqFinished++
			if reqFinished == postersNum {
				break loop
			}
		case <-timer.C:
			t.Fatal("Timeout")
		}
	}
}

func TestGetNowPlayingConcurrent(t *testing.T) {
	key, err := getKey()
	if err != nil {
		t.Fatal(err)
	}

	// В результате выполнения предыдущего теста может не остаться запросов.
	time.Sleep(APIRateLimitDur)

	client := NewClient(key, nil)
	start := make(chan struct{})
	result := make(chan error)
	var lineUp sync.WaitGroup
	lineUp.Add(apiRateLimit)
	for i := 0; i < apiRateLimit; i++ {
		go func() {
			lineUp.Done()
			<-start
			_, err := client.GetNowPlaying(testNowPlayingPage)
			result <- err
		}()
	}
	lineUp.Wait()
	close(start)
	reqFinished := 0
	timer := time.NewTimer(time.Second * 5)
loop:
	for {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
			reqFinished++
			if reqFinished == apiRateLimit {
				break loop
			}
		case <-timer.C:
			t.Fatal("Timeout")
		}
	}
}

func getKey() (string, error) {
	f, err := os.Open("key.txt")
	if err != nil {
		return "", errors.New("The MovieDB API key not found")
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return "", errors.New("The MovieDB API key not found")
	}

	key := string(data)
	key = strings.TrimSpace(key)
	return key, nil
}
