// Пакет sqlite реализует простой драйвер для работы с БД SQLite.
package sqlite

// #cgo CFLAGS: -I${SRCDIR}/lib
// #cgo LDFLAGS: -L${SRCDIR}/lib -lsqlite3
/*
#include <stdlib.h>
#include "sqlite3.h"
*/
import "C"
import (
	"errors"
	"unsafe"
)

var ErrGeneral = errors.New("SQLite error")
var ErrNegativeColumnNum = errors.New("Column number is negative")
var ErrNoData = errors.New("No data")
var ErrColumnNum = errors.New("Column number exceeds columns count in a result set")
var ErrColumnNotInteger = errors.New("Column type in database isn't of INTEGER type")
var ErrColumnNotFloat = errors.New("Column type in database isn't of FLOAT type")
var ErrColumnNotText = errors.New("Column type in database isn't of TEXT type")
var ErrColumnNotBlob = errors.New("Column type in database isn't of BLOB type")
var ErrColumnType = errors.New("Unknown column type")
var ErrNullColumn = errors.New("Column is NULL")

// Conn представляет собой одно соединение с БД.
type Conn struct {
	db   *C.struct_sqlite3
	stmt *C.struct_sqlite3_stmt
}

// NewConn создаёт новое соединение с БД. filename является названием файла БД
// на диске. Если файла filename на диске нет, то NewConn создаёт его. Если
// есть, то он остаётся без изменений. По окончанию работ для соединения должен
// быть вызван метод Close.
func NewConn(filename string) (*Conn, error) {
	filenameCStr := C.CString(filename)
	defer C.free(unsafe.Pointer(filenameCStr))
	conn := new(Conn)
	resultCode := C.sqlite3_open(filenameCStr, &conn.db)
	if resultCode == C.SQLITE_OK {
		return conn, nil
	}
	conn.Close()
	return nil, ErrGeneral
}

// Exec выполняет запрос query.
// Если запрос выполнен успешно и завершён, то возвращается true, nil.
// Если запрос выполнен успешно, но ещё не завершён, то возвращается false, nil
// и вызовом Next можно продвигаться дальше по запросу, но при этом первый
// результат должен быть извлечён до первого вызова Next.
// Если запрос выполнен неуспешно, то возвращается false и ошибка.
func (c *Conn) Exec(query string) (bool, error) {
	queryCStr := C.CString(query)
	defer C.free(unsafe.Pointer(queryCStr))
	resultCode := C.sqlite3_prepare_v2(c.db, queryCStr, C.int(-1), &c.stmt, nil)
	if resultCode == C.SQLITE_OK {
		return c.Next()
	}
	C.sqlite3_finalize(c.stmt)
	return false, ErrGeneral
}

// Next продвигается дальше по запросу, который был выполнен методом Exec.
// Если ошибок нет, то возвращается статус завершения запроса (true - завершён,
// false - нет) и nil. Иначе возвращается false и ошибка.
func (c *Conn) Next() (bool, error) {
	resultCode := C.sqlite3_step(c.stmt)
	switch resultCode {
	case C.SQLITE_DONE:
		C.sqlite3_finalize(c.stmt)
		c.stmt = nil
		return true, nil

	case C.SQLITE_ROW:
		return false, nil

	default:
		C.sqlite3_finalize(c.stmt)
		c.stmt = nil
		return false, ErrGeneral
	}
}

// Column позволяет получить значение колонки с номером colNum. Счёт колонок
// начинается с 0. Значение колонки записывается в value. Динамический тип
// value должен быть *int64, *float64, *string или *[]byte для получения
// соответственно значения целочисленной, вещественной, строковой или бинарной
// колонки. Если динамический тип value и тип колонки не совпадают, то
// возвращается ошибка. Если колонка равна NULL, то возвращается ошибка
// ErrNullColumn.
func (c *Conn) Column(colNum int, value interface{}) error {
	if colNum < 0 {
		return ErrNegativeColumnNum
	}

	colsCount := C.sqlite3_data_count(c.stmt)
	if colsCount <= 0 {
		return ErrNoData
	}

	if colNum >= int(colsCount) {
		return ErrColumnNum
	}

	colNumC := C.int(colNum)
	colType := C.sqlite3_column_type(c.stmt, colNumC)
	if colType == C.SQLITE_NULL {
		return ErrNullColumn
	}

	switch value := value.(type) {
	case *int64:
		if colType != C.SQLITE_INTEGER {
			return ErrColumnNotInteger
		}
		*value = int64(C.sqlite3_column_int64(c.stmt, colNumC))

	case *float64:
		if colType != C.SQLITE_FLOAT {
			return ErrColumnNotFloat
		}
		*value = float64(C.sqlite3_column_double(c.stmt, colNumC))

	case *string:
		if colType != C.SQLITE_TEXT {
			return ErrColumnNotText
		}
		cStr := C.sqlite3_column_text(c.stmt, colNumC)
		*value = C.GoString((*C.char)(unsafe.Pointer(cStr)))

	case *[]byte:
		if colType != C.SQLITE_BLOB {
			return ErrColumnNotBlob
		}
		blob := C.sqlite3_column_blob(c.stmt, colNumC)
		blobSize := C.sqlite3_column_bytes(c.stmt, colNumC)
		*value = C.GoBytes(blob, blobSize)

	default:
		return ErrColumnType
	}

	return nil
}

// Close закрывает соединение с БД. Должен быть объязательно вызван после
// окончания работы с соединением, чтобы не было утечек ресурсов.
func (c *Conn) Close() error {
	if c.stmt != nil {
		C.sqlite3_finalize(c.stmt)
		c.stmt = nil
	}
	if c.db == nil {
		return nil
	}
	resultCode := C.sqlite3_close(c.db)
	if resultCode == C.SQLITE_OK {
		return nil
	}
	return ErrGeneral
}

// Version возвращает версию библиотеки SQLite.
func Version() string {
	return C.GoString(C.sqlite3_libversion())
}
