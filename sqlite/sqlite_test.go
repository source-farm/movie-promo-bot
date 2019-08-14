package sqlite

import (
	"os"
	"testing"
)

func TestCreateDB(t *testing.T) {
	dbName := "test.db"
	conn, err := NewConn(dbName)
	if err != nil {
		t.Fatal("Cannot create database")
	}
	conn.Close()
	os.Remove(dbName)
}
