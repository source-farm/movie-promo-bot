package sqlite

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

var dbName = "test.db"

func TestCreateDB(t *testing.T) {
	defer cleanup()

	conn, err := NewConn(dbName)
	if err != nil {
		t.Fatal("Cannot create database")
	}
	if conn.Close() != nil {
		t.Fatal("Cannot close connection")
	}
}

func TestCreateTable(t *testing.T) {
	defer cleanup()

	conn, err := NewConn(dbName)
	if err != nil {
		t.Fatal("Cannot create database")
	}
	defer conn.Close()
	finished, err := conn.Exec("CREATE TABLE test(id INTEGER);")
	if !finished || err != nil {
		t.Fatal("Cannot create table")
	}
}

func TestMegaQuery(t *testing.T) {
	defer cleanup()
	N := 1000000

	conn, err := NewConn(dbName)
	if err != nil {
		t.Fatal("Cannot create database")
	}
	defer conn.Close()
	finished, err := conn.Exec("CREATE TABLE test(id INTEGER);")
	if !finished || err != nil {
		t.Fatal("Cannot create table")
	}

	queryBuilder := strings.Builder{}
	queryBuilder.Write([]byte("INSERT INTO test(id) VALUES (0)"))
	for i := 1; i < N; i++ {
		queryBuilder.Write([]byte(fmt.Sprintf(",(%d)", i)))
	}
	queryBuilder.Write([]byte(";"))

	megaQuery := queryBuilder.String()
	finished, err = conn.Exec(megaQuery)
	if !finished || err != nil {
		t.Fatal("Cannot insert values")
	}

	var n, i int64
	finished, err = conn.Exec("SELECT id FROM test ORDER BY id ASC;")
	for !finished {
		err = conn.Column(0, &n)
		if err != nil {
			t.Fatal("Error while getting column value")
		}
		if n != i {
			t.Fatalf("Value %d not found in a table", i)
		}
		i++

		finished, err = conn.Next()
		if err != nil {
			t.Fatal("Error on stepping to next row")
		}
	}

	if i != int64(N) {
		t.Fatalf("Table must have exactly %d rows", N)
	}
}

func TestColumnTypes(t *testing.T) {
	defer cleanup()

	conn, err := NewConn(dbName)
	if err != nil {
		t.Fatal("Cannot create database")
	}
	defer conn.Close()
	finished, err := conn.Exec("CREATE TABLE test(n INTEGER, f FLOAT, t TEXT, b BLOB);")
	if !finished || err != nil {
		t.Fatal("Cannot create table")
	}

	finished, err = conn.Exec("INSERT INTO test(n, f, t, b) VALUES(42, 3.14159, 'foo', X'DEADBEEF');")
	if !finished || err != nil {
		t.Fatal("Cannot insert values")
	}

	finished, err = conn.Exec("SELECT n, f, t, b FROM test;")
	if err != nil {
		t.Fatal("Cannot query data")
	}

	var n int64
	err = conn.Column(0, &n)
	if err != nil || n != 42 {
		t.Fatal("Cannot query integer column")
	}

	var f float64
	err = conn.Column(1, &f)
	if err != nil || f != 3.14159 {
		t.Fatal("Cannot query float column")
	}

	var s string
	err = conn.Column(2, &s)
	if err != nil || s != "foo" {
		t.Fatal("Cannot query string column")
	}

	var b []byte
	deadbeef := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	err = conn.Column(3, &b)
	if err != nil || !reflect.DeepEqual(b, deadbeef) {
		t.Fatal("Cannot query binary column")
	}
}

func cleanup() {
	_, err := os.Stat(dbName)
	if err == nil {
		os.Remove(dbName)
	}
}
