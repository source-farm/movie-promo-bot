package sqlite

import (
	"fmt"
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

	// Добавляем в таблицу одну строку.
	stmt, err = conn.Prepare("INSERT INTO test(n, f, t, b) VALUES(?1, ?2, ?3, ?4);")
	if err != nil {
		t.Fatal(err)
	}
	err = stmt.Exec(int64(1), 1.0, "foo", []byte{0xDE, 0xAD, 0xBE, 0xEF})
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

	err = conn.Exec("CREATE TABLE test(n INTEGER, f FLOAT, t TEXT, b BLOB);")
	if err != nil {
		t.Fatal(err)
	}

	// Заполняем таблицу данными.
	stmt, err := conn.Prepare("INSERT INTO test(n, f, t, b) VALUES(?1, ?2, ?3, ?4);")
	if err != nil {
		t.Fatal(err)
	}
	N := 100
	for i := 1; i <= N; i++ {
		err := stmt.Exec(int64(i), 1.0, "foo", []byte{0xDE, 0xAD, 0xBE, 0xEF})
		if err != nil {
			t.Fatal(err)
		}
	}
	err = stmt.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Проверяем, что всё записалось.
	stmt, err = conn.Prepare("SELECT n, f, t, b FROM test ORDER BY n ASC;")
	if err != nil {
		t.Fatal(err)
	}
	iter, err := stmt.Query()
	if err != nil {
		t.Fatal(err)
	}

	deadbeef := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	rowNum := 0
	for iter.Next() {
		if iter.Err() != nil {
			t.Fatal(iter.Err())
		}

		var n int64
		var f float64
		var s string
		var b []byte
		err := iter.Scan(&n, &f, &s, &b)
		if err != nil {
			t.Fatal(err)
		}
		rowNum++
		if n != int64(rowNum) || f != 1.0 || s != "foo" || !reflect.DeepEqual(b, deadbeef) {
			t.Fatal("Unexpected value")
		}
	}
	if iter.Err() != nil {
		t.Fatal(iter.Err())
	}
	if rowNum != N {
		fmt.Println(rowNum)
		t.Fatal("Not all rows fetched")
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
