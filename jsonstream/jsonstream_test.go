package jsonstream

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValueJSON(t *testing.T) {
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
	scanner = NewScanner()
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
	scanner = NewScanner()
	scanner.SearchFor(&f)
	err = scanner.Find(strings.NewReader("4.2"))
	if f != 4.2 {
		t.Fatal(err)
	}

	// Number
	var number json.Number
	scanner = NewScanner()
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
	scanner = NewScanner()
	scanner.SearchFor(&s)
	err = scanner.Find(strings.NewReader(`"foo"`))
	if s != "foo" {
		t.Fatal(err)
	}
}
