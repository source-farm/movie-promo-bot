package themoviedb

import (
	"bot/iso6391"
	"bytes"
	"errors"
	"image/jpeg"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

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
	fightClubID := 550
	movie, err := client.GetMovie(fightClubID)
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
	fightClubID := 550
	movie, err := client.GetMovie(fightClubID)
	if err != nil {
		t.Fatal(err)
	}

	poster, ok := movie.Poster[iso6391.En]
	if !ok {
		t.Fatal("English poster not found")
	}
	data, err := client.GetPoster(poster.Path)
	if !ok {
		t.Fatal(err)
	}

	_, err = jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("Cannot decode poster")
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
