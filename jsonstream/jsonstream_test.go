package jsonstream

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSingleValueJSON(t *testing.T) {
	// null
	ptr := new(int)
	scanner := NewScanner()
	scanner.SearchFor(&ptr)
	err := scanner.Find(strings.NewReader("null"))
	if ptr != nil {
		t.Fatal(err)
	}

	// bool
	b := false
	scanner.Reset()
	scanner.SearchFor(&b)
	err = scanner.Find(strings.NewReader("true"))
	if !b {
		t.Fatal(err)
	}
	b = true
	err = scanner.Find(strings.NewReader("false"))
	if b {
		t.Fatal(err)
	}

	// float64
	f := 0.0
	scanner.Reset()
	scanner.SearchFor(&f)
	err = scanner.Find(strings.NewReader("4.2"))
	if f != 4.2 {
		t.Fatal(err)
	}

	// Number
	var number json.Number
	scanner.Reset()
	scanner.SearchFor(&number)
	err = scanner.Find(strings.NewReader("42"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := number.Int64()
	if n != 42 {
		t.Fatal(err)
	}

	// string
	var s string
	scanner.Reset()
	scanner.SearchFor(&s)
	err = scanner.Find(strings.NewReader(`"foo"`))
	if s != "foo" {
		t.Fatal(err)
	}
}

func TestSingleValueExtract(t *testing.T) {
	type Transport struct {
		Private bool
		Public  []string
	}
	type Town struct {
		Name      string
		Age       int
		Transport Transport
	}

	var town = Town{
		Name: "Marine",
		Age:  221,
		Transport: Transport{
			Private: false,
			Public:  []string{"bus", "metro", "taxi", "tramway"},
		},
	}
	// town в виде JSON'а
	//	{
	//		"Name": "Marine",
	//		"Age": 221
	//		"Transport": {
	//			"Private": false,
	//			"Public": ["bus", "metro", "taxi", "tramway"]
	//		}
	//	}

	testJSON, err := json.Marshal(town)
	if err != nil {
		t.Fatal(err)
	}

	// Извлечение всего JSON'а.
	scanner := NewScanner()
	var extractedTown Town
	scanner.SearchFor(&extractedTown)
	err = scanner.Find(bytes.NewReader(testJSON))
	if !reflect.DeepEqual(town, extractedTown) {
		t.Fatal(err)
	}

	// Извлечение одного значения на первом уровне вложенности.
	var name string
	scanner.Reset()
	scanner.SearchFor(&name, "Name")
	err = scanner.Find(bytes.NewReader(testJSON))
	if name != "Marine" {
		t.Fatal(name)
	}

	// Извлечение одного значения на втором уровне вложенности.
	private := true
	scanner.Reset()
	scanner.SearchFor(&private, "Transport", "Private")
	err = scanner.Find(bytes.NewReader(testJSON))
	if private {
		t.Fatal(err)
	}

	// Извлечение массива на втором уровне вложенности.
	public := []string{}
	scanner.Reset()
	scanner.SearchFor(&public, "Transport", "Public")
	err = scanner.Find(bytes.NewReader(testJSON))
	if !reflect.DeepEqual(town.Transport.Public, public) {
		t.Fatal(err)
	}
}

func TestMultipleValuesExtract(t *testing.T) {
	type Size struct {
		Width  int
		Height int
	}
	type Painting struct {
		Name   string
		Artist string
		Year   int
		Size   Size
	}

	painting := Painting{
		Name:   "Basket of Fruit",
		Artist: "Caravaggio",
		Year:   1599,
		Size: Size{
			Width:  64,
			Height: 46,
		},
	}
	// painting в виде JSON'а
	//	{
	//		"Name": "Basket of Fruit",
	//		"Artist": "Caravaggio",
	//		"Year": 1599,
	//		"Size": {
	//			"Width": 64,
	//			"Height": 46
	//		}
	//	}

	testJSON, err := json.Marshal(painting)
	if err != nil {
		t.Fatal(err)
	}

	var artist string
	var width int
	scanner := NewScanner()
	scanner.SearchFor(&artist, "Artist")
	scanner.SearchFor(&width, "Size", "Width")
	err = scanner.Find(bytes.NewReader(testJSON))
	if err != nil {
		t.Fatal(err)
	}
	if artist != painting.Artist || width != painting.Size.Width {
		t.Fatal(err)
	}
}
