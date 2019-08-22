package themoviedb

import (
	"os"
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
