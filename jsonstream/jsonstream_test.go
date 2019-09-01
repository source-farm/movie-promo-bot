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
	// town в виде JSON'а:
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
		Name     string
		Artist   string
		Year     int
		Location string
		Size     Size
	}

	painting := Painting{
		Name:     "Basket of Fruit",
		Artist:   "Caravaggio",
		Year:     1599,
		Location: "Biblioteca Ambrosiana",
		Size: Size{
			Width:  64,
			Height: 46,
		},
	}
	// painting в виде JSON'а:
	//	{
	//		"Name": "Basket of Fruit",
	//		"Artist": "Caravaggio",
	//		"Year": 1599,
	//		"Location": "Biblioteca Ambrosiana",
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

	scanner.Reset()
	var depth int
	scanner.SearchFor(&depth, "Size", "Depth")
	err = scanner.Find(bytes.NewReader(testJSON))
	if err == nil {
		t.Fatal("want error but got nil")
	}
}

func TestOverlappingPaths(t *testing.T) {
	type Map struct {
		Secret           string
		TreasureLocation string
	}
	type Captain struct {
		Nickname    string
		FavoritePet string
		Map         Map
	}
	type Ship struct {
		Name    string
		Pirate  bool
		Captain Captain
	}

	ship := Ship{
		Name:   "Wind master",
		Pirate: true,
		Captain: Captain{
			Nickname:    "Woodleg",
			FavoritePet: "Parrot",
			Map: Map{
				Secret:           "Sink map in water",
				TreasureLocation: "Gull island, Redbeard's shack",
			},
		},
	}
	// ship в виде JSON'а:
	//	{
	//		"Name": "Wind master",
	//		"Pirate": true,
	//		"Captain": {
	//			"Nickname": "Woodleg",
	//			"FavoritePet": "Parrot",
	//			"Map": {
	//				"Secret": "Sink in water",
	//				"TreasureLocation": "Gull island, Redbeard's shack"
	//			}
	//		}
	//	}

	testJSON, err := json.Marshal(ship)
	if err != nil {
		t.Fatal(err)
	}

	// Проверка невозможности добавления продолжения уже существующего пути.
	decodedShip := Ship{}
	var shipName string
	scanner := NewScanner()
	err = scanner.SearchFor(&decodedShip)
	if err != nil {
		t.Fatal(err)
	}
	err = scanner.SearchFor(&shipName, "Name")
	if err == nil {
		t.Fatal("want error but got nil")
	}

	// Проверка невозможности добавления продолжения уже существующего пути.
	var captain Captain
	var nickname string
	scanner.Reset()
	err = scanner.SearchFor(&captain, "Captain")
	if err != nil {
		t.Fatal(err)
	}
	err = scanner.SearchFor(&nickname, "Captain", "Nickname")
	if err == nil {
		t.Fatal("want error but got nil")
	}

	// Проверка вытеснения более общим путём продолжений этого общего пути.
	captain = Captain{}
	nickname = ""
	var favoritePet string
	scanner.Reset()
	err = scanner.SearchFor(&nickname, "Captain", "Nickname")
	if err != nil {
		t.Fatal(err)
	}
	err = scanner.SearchFor(&favoritePet, "Captain", "FavoritePet")
	if err != nil {
		t.Fatal(err)
	}
	err = scanner.SearchFor(&captain, "Captain")
	if err != nil {
		t.Fatal(err)
	}
	err = scanner.Find(bytes.NewReader(testJSON))
	if err != nil {
		t.Fatal(err)
	}
	if nickname != "" || favoritePet != "" {
		t.Fatal("decoded unexpected values")
	}
	if !reflect.DeepEqual(captain, ship.Captain) {
		t.Fatal("decode error")
	}

	// Проверка двойного добавления одного и того же пути.
	map1 := Map{}
	map2 := Map{}
	scanner.Reset()
	err = scanner.SearchFor(&map1, "Captain", "Map")
	if err != nil {
		t.Fatal(err)
	}
	err = scanner.SearchFor(&map2, "Captain", "Map")
	if err != nil {
		t.Fatal(err)
	}
	err = scanner.Find(bytes.NewReader(testJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(map1, Map{}) || !reflect.DeepEqual(map2, ship.Captain.Map) {
		t.Fatal("add path twice error")
	}
}

func TestFilter(t *testing.T) {
	// Проверка пустого фильтра.
	nums := []int{1, 2, 3}
	testJSON, err := json.Marshal(nums)
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner()
	var decodedNums []int
	scanner.SearchFor(&decodedNums)
	filter := func(v interface{}) bool {
		return true
	}
	err = scanner.SetFilter(filter)
	if err != nil {
		t.Fatal(err)
	}
	err = scanner.Find(bytes.NewReader(testJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodedNums, nums) {
		t.Fatal("decode error")
	}

	/*
		// Проверка фильтрации массива строк на первом уровне вложенности.
		type Dict struct {
			Author string
			Words  []string
		}
		dict := Dict{
			Author: "Mr. Claydon",
			Words:  []string{"fly", "home", "freeze", "ace", "zero"},
		}
		testJSON, err = json.Marshal(dict)
		if err != nil {
			t.Fatal(err)
		}
		filter = func(v interface{}) bool {
			word, ok := v.(string)
			if !ok {
				return false
			}
			return len(word) <= 4
		}
		var wordsFiltered []string
		for _, word := range dict.Words {
			if filter(word) {
				wordsFiltered = append(wordsFiltered, word)
			}
		}
		scanner.Reset()
		var wordsDecoded []string
		scanner.SearchFor(&wordsDecoded, "Words")
		err = scanner.SetFilter(filter, "Words")
		if err != nil {
			t.Fatal(err)
		}
		err = scanner.Find(bytes.NewReader(testJSON))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(wordsFiltered, wordsDecoded) {
			t.Fatal("decode error")
		}
	*/

	// Проверка фильтрации массива объектов на первом уровне вложенности.
	type HourTemp struct {
		Hour int
		Temp float64
	}
	dayTemp := []HourTemp{
		{0, 12.1},
		{1, 15.2},
		{2, 10.8},
		{3, 17.8},
		{4, 18.0},
		{5, 18.3},
		{6, 18.4},
		{7, 18.7},
		{8, 20.3},
	}
	testJSON, err = json.Marshal(dayTemp)
	if err != nil {
		t.Fatal(err)
	}
	filter = func(v interface{}) bool {
		hourTemp, ok := v.(HourTemp)
		if !ok {
			return false
		}
		return hourTemp.Temp > 17.0
	}
	var dayTempFiltered []HourTemp
	for _, hourTemp := range dayTemp {
		if filter(hourTemp) {
			dayTempFiltered = append(dayTempFiltered, hourTemp)
		}
	}
	scanner.Reset()
	var dayTempDecoded []HourTemp
	scanner.SearchFor(&dayTempDecoded)
	err = scanner.SetFilter(filter)
	if err != nil {
		t.Fatal(err)
	}
	err = scanner.Find(bytes.NewReader(testJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dayTempFiltered, dayTempDecoded) {
		t.Fatal("decode error")
	}
}
