package sqlite

import (
	"os"
	"reflect"
	"testing"
)

var dbName = "test.db"

func TestCreateDB(t *testing.T) {
	defer cleanup()

	conn, err := NewConn(dbName)
	if err != nil {
		t.Fatal(err)
	}
	err = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateTable(t *testing.T) {
	defer cleanup()

	conn, err := NewConn(dbName)
	if err != nil {
		t.Fatal(err)
	}

	err = conn.Exec("CREATE TABLE test(id INTEGER);")
	if err != nil {
		t.Fatal(err)
	}

	err = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestStmt(t *testing.T) {
	defer cleanup()

	conn, err := NewConn(dbName)
	if err != nil {
		t.Fatal(err)
	}

	// Создаём таблицу.
	stmt, err := conn.Prepare("CREATE TABLE test(n INTEGER, f REAL, t TEXT, b BLOB);")
	if err != nil {
		t.Fatal(err)
	}
	err = stmt.Exec()
	if err != nil {
		t.Fatal(err)
	}
	err = stmt.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Заполняем таблицу данными.
	stmt, err = conn.Prepare("INSERT INTO test(n, f, t, b) VALUES(?1, ?2, ?3, ?4);")
	if err != nil {
		t.Fatal(err)
	}
	err = stmt.Exec(1, 1.0, "foo", []byte{0xDE, 0xAD, 0xBE, 0xEF})
	if err != nil {
		t.Fatal(err)
	}
	err = stmt.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	defer cleanup()

	conn, err := NewConn(dbName)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	err = conn.Exec("CREATE TABLE test(n INTEGER, f FLOAT, t TEXT, b BLOB);")
	if err != nil {
		t.Fatal(err)
	}

	err = conn.Exec("INSERT INTO test(n, f, t, b) VALUES(1, 1.0, 'foo', X'DEADBEEF');")
	if err != nil {
		t.Fatal(err)
	}

	stmt, err := conn.Prepare("SELECT n, f, t, b FROM test;")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := stmt.Query()
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	var f float64
	var s string
	var b []byte
	if !rows.Next() {
		t.Fatal("Expected data, got nothing")
	}
	err = rows.Scan(&n, &f, &s, &b)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("Cannot query integer column")
	}
	if f != 1.0 {
		t.Fatal("Cannot query float column")
	}
	if s != "foo" {
		t.Fatal("Cannot query string column")
	}
	deadbeef := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !reflect.DeepEqual(b, deadbeef) {
		t.Fatal("Cannot query binary column")
	}

	if rows.Next() {
		t.Fatal("Expected nothing, got data")
	}

	err = rows.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = stmt.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func cleanup() {
	_, err := os.Stat(dbName)
	if err == nil {
		os.Remove(dbName)
	}
}
