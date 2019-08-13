package main

import (
	"bot/sqlite"
	"fmt"
	"log"
)

func foo(v interface{}) {
	switch v.(type) {
	case int:
		fmt.Println("Just int")
	case *int:
		v := v.(*int)
		*v = 42
	case *[]byte:
		v := v.(*[]byte)
		*v = []byte("Hello")
		fmt.Println("Just []byte")
	}
}

func main() {
	/*
		var n int
		foo(n)
		foo(&n)
		var blob []byte
		foo(&blob)
		fmt.Println(string(blob))
	*/

	log.SetFlags(0)

	fmt.Println(sqlite.Version())

	conn, err := sqlite.NewConn("movie.db")
	if err != nil {
		log.Fatal(err)
	}

	/*
		_, err = conn.Exec("CREATE TABLE movie(id INTEGER, r REAL);")
		_, err = conn.Exec("INSERT INTO movie(id, r) VALUES (1, 3.1415);")
		_, err = conn.Exec("INSERT INTO movie(id, r) VALUES (2, 2.18281828);")
	*/
	finished, err := conn.Exec("SELECT * FROM movie ORDER BY id;")
	if err != nil {
		log.Fatal(err)
	}
	for !finished {
		var n int64
		var f float64
		var s string
		var b []byte
		conn.Column(0, &n)
		conn.Column(1, &f)
		conn.Column(2, &s)
		err = conn.Column(3, &b)
		fmt.Println(n, f, s, b, err)
		finished, err = conn.Next()
	}
	// _, err = conn.Exec("DROP TABLE movie;")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	err = conn.Column(0, 1)
	fmt.Println(err)

	err = conn.Close()
	if err != nil {
		log.Fatal(err)
	}
}
