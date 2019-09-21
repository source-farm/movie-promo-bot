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
	"fmt"
	"reflect"
	"strconv"
	"unsafe"
)

var (
	ErrConnDone       = errors.New("sqlite: connection is already closed")
	ErrStmtDone       = errors.New("sqlite: statement is already closed")
	ErrNoRows         = errors.New("sqlite: no rows in result set")
	ErrColumnMismatch = errors.New("sqlite: columns count mismatch")
	ErrColumnType     = errors.New("sqlite: unsupported column type")
)

// Rows используется для итерации по результату запроса. Rows не является
// потоко-безопасным.
type Rows struct {
	stmt *Stmt
	done bool
	err  error
}

// Next переходит к следующей строке запроса, которую можно вычитать с помощью
// метода Scan. Если к следующей строке удалось перейти, то возвращается true.
// Если при переходе возникла ошибка (можно получить вызовом метода Err) или
// больше не осталось строк (Err возвращает nil), то возвращается false.
func (r *Rows) Next() bool {
	if r.done {
		return false
	}

	if r.stmt.stmt != nil {
		resCode := C.sqlite3_step(r.stmt.stmt)
		err := resultCode2GoError(resCode)
		switch err {
		case SQLiteDone:
			r.done = true
			r.err = nil

		case SQLiteRow:
			r.err = nil
			return true

		default:
			r.err = err
		}
	} else {
		// Пока Rows не дошёл до конца был закрыт Stmt, из которого и был
		// получен Rows.
		r.err = ErrStmtDone
	}

	return false
}

// Err возвращает последнюю ошибку, которая могла возникнуть после вызова Next.
func (r *Rows) Err() error {
	return r.err
}

// Scan сканирует по очереди колонки в текущей строке в указатели из dest.
// Указатели могут быть типа *int64, *float64, *string, *[]byte для получения
// значения целочисленной, вещественной, строковой или бинарной колонки
// соответственно.
func (r *Rows) Scan(dest ...interface{}) error {
	if r.stmt.stmt == nil {
		return ErrStmtDone
	}

	colsCount := int(C.sqlite3_data_count(r.stmt.stmt))
	if colsCount <= 0 {
		return ErrNoRows
	}

	if colsCount != len(dest) {
		return ErrColumnMismatch
	}

	for i := range dest {
		colNum := C.int(i)
		colType := C.sqlite3_column_type(r.stmt.stmt, colNum)
		switch value := dest[i].(type) {
		case *int64:
			if colType != C.SQLITE_INTEGER {
				return errors.New("sqlite: column " + strconv.Itoa(i) + " isn't INTEGER")
			}
			*value = int64(C.sqlite3_column_int64(r.stmt.stmt, colNum))

		case *float64:
			if colType != C.SQLITE_FLOAT {
				return errors.New("sqlite: column " + strconv.Itoa(i) + " isn't REAL")
			}
			*value = float64(C.sqlite3_column_double(r.stmt.stmt, colNum))

		case *string:
			if colType != C.SQLITE_TEXT {
				return errors.New("sqlite: column " + strconv.Itoa(i) + " isn't TEXT")
			}
			cStr := C.sqlite3_column_text(r.stmt.stmt, colNum)
			*value = C.GoString((*C.char)(unsafe.Pointer(cStr)))

		case *[]byte:
			if colType != C.SQLITE_BLOB {
				return errors.New("sqlite: column " + strconv.Itoa(i) + " isn't BLOB")
			}
			blob := C.sqlite3_column_blob(r.stmt.stmt, colNum)
			blobSize := C.sqlite3_column_bytes(r.stmt.stmt, colNum)
			*value = C.GoBytes(blob, blobSize)

		default:
			return ErrColumnType
		}
	}

	return nil
}

// Row используется для получения первой строки запроса. Row не является
// потоко-безопасным.
type Row struct {
	rows *Rows
	err  error
}

// Scan сканирует по очереди колонки из первой строки в dest. Более подробно
// можно прочитать в описании метода Rows.Scan.
func (r *Row) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}

	return r.rows.Scan(dest...)
}

// Stmt - это скомпилированное SQL-предложение (SQL statement). Stmt не
// является потоко-безопасным.
type Stmt struct {
	stmt *C.struct_sqlite3_stmt
}

// Exec выполняет скомпилированное SQL-предложение. Аргументы args
// подставляются в предложение перед его выполнением.
func (s *Stmt) Exec(args ...interface{}) error {
	if s.stmt == nil {
		return ErrStmtDone
	}

	err := s.bind(args...)
	if err != nil {
		return err
	}

	resCode := C.sqlite3_step(s.stmt)
	err = resultCode2GoError(resCode)
	if err == SQLiteDone {
		return nil
	}

	return err
}

// Query выполняет скомпилированный запрос. Аргументы args подставляются в
// запрос перед его выполнением. Если нет ошибок, то *Rows позволяет получить
// все строки выполненного запроса.
func (s *Stmt) Query(args ...interface{}) (*Rows, error) {
	if s.stmt == nil {
		return nil, ErrStmtDone
	}

	err := s.bind(args...)
	if err != nil {
		return nil, err
	}

	rows := Rows{stmt: s, done: false, err: nil}
	return &rows, nil
}

// QueryRow выполняет скомпилированный запрос. Аргументы args подставляются в
// запрос перед его выполнением. Если во время выполнения запроса возникнет
// ошибка, то её можно получить вызовом Scan для возвращаемого *Row, который
// никогда не равен nil. Если результатом выполнения запроса является пустое
// множество строк, то Scan для *Row возвращает ErrNoRows. Иначе Scan сканирует
// первую строку и игнорирует остальные.
func (s *Stmt) QueryRow(args ...interface{}) *Row {
	rows, err := s.Query(args...)
	switch {
	case err != nil:
		return &Row{rows: nil, err: err}

	case rows.Next():
		return &Row{rows: rows, err: nil}

	case rows.Err() == nil:
		return &Row{rows: nil, err: ErrNoRows}

	default:
		return &Row{rows: nil, err: rows.Err()}
	}
}

// bind подставляет аргументы args с SQL-предложение. args может содержать
// только значения типа int64, float64, string или []byte.
func (s *Stmt) bind(args ...interface{}) error {
	if s.stmt == nil {
		return ErrStmtDone
	}

	// Сбрасываем s.stmt в начало.
	resCode := C.sqlite3_reset(s.stmt)
	err := resultCode2GoError(resCode)
	if err != nil {
		return err
	}

	// Очищаем подставновки (sqlite3_reset этого не делает).
	resCode = C.sqlite3_clear_bindings(s.stmt)
	err = resultCode2GoError(resCode)
	if err != nil {
		return err
	}

	for i, arg := range args {
		argType := reflect.TypeOf(arg)
		switch argType.Kind() {
		case reflect.Int64:
			resCode = C.sqlite3_bind_int64(s.stmt, C.int(i+1), C.sqlite3_int64(arg.(int64)))

		case reflect.Float64:
			resCode = C.sqlite3_bind_double(s.stmt, C.int(i+1), C.double(arg.(float64)))

		case reflect.String:
			str := arg.(string)
			cStr := C.CString(str)
			defer C.free(unsafe.Pointer(cStr))
			// C.SQLITE_TRANSIENT приводит к копированию строки во внутреннюю
			// память SQLite. Так и не смог понять из документации
			// (https://www.sqlite.org/c3ref/bind_blob.html), когда безопасно
			// освобождать передаваемый в SQLite указатель на строку cStr.
			resCode = C.sqlite3_bind_text(s.stmt, C.int(i+1), cStr, C.int(len(str)), C.SQLITE_TRANSIENT)

		case reflect.Slice:
			if argType.Elem().Kind() != reflect.Uint8 { // не []byte
				argTypeStr := fmt.Sprintf("%T", arg)
				return errors.New("sqlite: unsupported type (" + argTypeStr + ")")
			}
			data := arg.([]byte)
			cData := C.CBytes(data)
			defer C.free(unsafe.Pointer(cData))
			// C.SQLITE_TRANSIENT приводит к копированию данных слайса во
			// внутреннюю память SQLite. Так и не смог понять из документации
			// (https://www.sqlite.org/c3ref/bind_blob.html), когда безопасно
			// освобождать передаваемый в SQLite указатель на данные cData.
			resCode = C.sqlite3_bind_blob(s.stmt, C.int(i+1), cData, C.int(len(data)), C.SQLITE_TRANSIENT)

		default:
			argTypeStr := fmt.Sprintf("%T", arg)
			return errors.New("sqlite: unsupported type (" + argTypeStr + ")")
		}
		err := resultCode2GoError(resCode)
		if err != nil {
			return err
		}
	}

	return nil
}

// Close освобождает ресурсы, выделенные под SQL-предложение.
func (s *Stmt) Close() error {
	if s.stmt == nil {
		return nil
	}

	resCode := C.sqlite3_finalize(s.stmt)
	err := resultCode2GoError(resCode)
	if err != nil {
		return err
	}
	s.stmt = nil

	return nil
}

// Conn - это одно соединение с БД. Conn не является потоко-безопасным.
type Conn struct {
	db *C.struct_sqlite3
}

// NewConn создаёт новое соединение с БД. dbFilename является названием файла
// БД на диске. Если файла dbFilename на диске нет, то NewConn создаёт его.
// Если есть, то он остаётся без изменений. По окончанию работ для соединения
// должен быть вызван метод Close.
func NewConn(dbFilename string) (*Conn, error) {
	dbFilenameCStr := C.CString(dbFilename)
	defer C.free(unsafe.Pointer(dbFilenameCStr))
	conn := new(Conn)
	resCode := C.sqlite3_open(dbFilenameCStr, &conn.db)
	err := resultCode2GoError(resCode)
	if err != nil {
		C.sqlite3_close(conn.db)
		return nil, err
	}
	return conn, nil
}

// Exec выполняет запрос query.
func (c *Conn) Exec(query string) error {
	if c.db == nil {
		return ErrConnDone
	}

	queryCStr := C.CString(query)
	defer C.free(unsafe.Pointer(queryCStr))
	resCode := C.sqlite3_exec(c.db, queryCStr, nil, nil, nil)
	err := resultCode2GoError(resCode)
	if err != nil {
		return err
	}

	return nil
}

// Prepare компилирует запрос query.
func (c *Conn) Prepare(query string) (*Stmt, error) {
	if c.db == nil {
		return nil, ErrConnDone
	}

	queryCStr := C.CString(query)
	defer C.free(unsafe.Pointer(queryCStr))
	stmt := new(Stmt)
	resCode := C.sqlite3_prepare_v2(c.db, queryCStr, C.int(-1), &stmt.stmt, nil)
	err := resultCode2GoError(resCode)
	if err != nil {
		return nil, err
	}

	return stmt, nil
}

// Close закрывает соединение с БД. Должен быть объязательно вызван после
// окончания работ с соединением, чтобы не было утечек ресурсов. Нельзя
// закрыть соединение пока есть не закрытые Stmt, которые были созданы из этого
// соединения.
func (c *Conn) Close() error {
	if c.db == nil {
		return nil
	}

	resCode := C.sqlite3_close(c.db)
	err := resultCode2GoError(resCode)
	if err != nil {
		return err
	}
	c.db = nil

	return nil
}

// Version возвращает версию библиотеки SQLite.
func Version() string {
	return C.GoString(C.sqlite3_libversion())
}
