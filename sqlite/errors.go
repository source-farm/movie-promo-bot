package sqlite

/*
#include "sqlite3.h"
*/
import "C"
import "errors"

// Ошибки библиотеки SQLite.
// SQLiteOK, SQLiteRow и SQLiteDone не являются ошибками, но они тоже являются
// переменными типа error для единообразия.
var (
	SQLiteOK      = error(nil)
	ErrGeneric    = errors.New("sqlite: generic error (SQLITE_ERROR)")
	ErrInternal   = errors.New("sqlite: internal logic error in SQLite (SQLITE_INTERNAL)")
	ErrPerm       = errors.New("sqlite: access permission denied (SQLITE_PERM)")
	ErrAbort      = errors.New("sqlite: callback routine requested an abort (SQLITE_ABORT)")
	ErrBusy       = errors.New("sqlite: the database file is locked (SQLITE_BUSY)")
	ErrLocked     = errors.New("sqlite: a table in the database is locked (SQLITE_LOCKED)")
	ErrNoMem      = errors.New("sqlite: a malloc() failed (SQLITE_NOMEM)")
	ErrReadOnly   = errors.New("sqlite: attempt to write a readonly database (SQLITE_READONLY)")
	ErrInterrupt  = errors.New("sqlite: operation terminated by sqlite3_interrupt() (SQLITE_INTERRUPT)")
	ErrIOErr      = errors.New("sqlite: some kind of disk I/O error occurred (SQLITE_IOERR)")
	ErrCorrupt    = errors.New("sqlite: the database disk image is malformed (SQLITE_CORRUPT)")
	ErrNotFound   = errors.New("sqlite: unknown opcode in sqlite3_file_control() (SQLITE_NOTFOUND)")
	ErrFull       = errors.New("sqlite: insertion failed because database is full (SQLITE_FULL)")
	ErrCantOpen   = errors.New("sqlite: unable to open the database file (SQLITE_CANTOPEN)")
	ErrProtocol   = errors.New("sqlite: database lock protocol error (SQLITE_PROTOCOL)")
	ErrEmpty      = errors.New("sqlite: internal use only (SQLITE_EMPTY)")
	ErrSchema     = errors.New("sqlite: the database schema changed (SQLITE_SCHEMA)")
	ErrTooBig     = errors.New("sqlite: string or BLOB exceeds size limit (SQLITE_TOOBIG)")
	ErrConstraint = errors.New("sqlite: abort due to constraint violation (SQLITE_CONSTRAINT)")
	ErrMismatch   = errors.New("sqlite: data type mismatch (SQLITE_MISMATCH)")
	ErrMisuse     = errors.New("sqlite: library used incorrectly (SQLITE_MISUSE)")
	ErrNoLFS      = errors.New("sqlite: uses OS features not supported on host (SQLITE_NOLFS)")
	ErrAuth       = errors.New("sqlite: authorization denied (SQLITE_AUTH)")
	ErrFormat     = errors.New("sqlite: not used (SQLITE_FORMAT)")
	ErrRange      = errors.New("sqlite: 2nd parameter to sqlite3_bind out of range (SQLITE_RANGE)")
	ErrNotDB      = errors.New("sqlite: file opened that is not a database file (SQLITE_NOTADB)")
	ErrNotice     = errors.New("sqlite: notifications from sqlite3_log() (SQLITE_NOTICE)")
	ErrWarning    = errors.New("sqlite: warnings from sqlite3_log() (SQLITE_WARNING)")
	SQLiteRow     = errors.New("sqlite: Sqlite3_step() has another row ready (SQLITE_ROW)")
	SQLiteDone    = errors.New("sqlite: Sqlite3_step() has finished executing (SQLITE_DONE)")
)

func resultCode2GoError(resCode C.int) error {
	switch resCode {
	case C.SQLITE_OK:
		return SQLiteOK
	case C.SQLITE_ERROR:
		return ErrGeneric
	case C.SQLITE_INTERNAL:
		return ErrInternal
	case C.SQLITE_PERM:
		return ErrPerm
	case C.SQLITE_ABORT:
		return ErrAbort
	case C.SQLITE_BUSY:
		return ErrBusy
	case C.SQLITE_LOCKED:
		return ErrLocked
	case C.SQLITE_NOMEM:
		return ErrNoMem
	case C.SQLITE_READONLY:
		return ErrReadOnly
	case C.SQLITE_INTERRUPT:
		return ErrInterrupt
	case C.SQLITE_IOERR:
		return ErrIOErr
	case C.SQLITE_CORRUPT:
		return ErrCorrupt
	case C.SQLITE_NOTFOUND:
		return ErrNotFound
	case C.SQLITE_FULL:
		return ErrFull
	case C.SQLITE_CANTOPEN:
		return ErrCantOpen
	case C.SQLITE_PROTOCOL:
		return ErrProtocol
	case C.SQLITE_EMPTY:
		return ErrEmpty
	case C.SQLITE_SCHEMA:
		return ErrSchema
	case C.SQLITE_TOOBIG:
		return ErrTooBig
	case C.SQLITE_CONSTRAINT:
		return ErrConstraint
	case C.SQLITE_MISMATCH:
		return ErrMismatch
	case C.SQLITE_MISUSE:
		return ErrMisuse
	case C.SQLITE_NOLFS:
		return ErrNoLFS
	case C.SQLITE_AUTH:
		return ErrAuth
	case C.SQLITE_FORMAT:
		return ErrFormat
	case C.SQLITE_RANGE:
		return ErrRange
	case C.SQLITE_NOTADB:
		return ErrNotDB
	case C.SQLITE_NOTICE:
		return ErrNotice
	case C.SQLITE_WARNING:
		return ErrWarning
	case C.SQLITE_ROW:
		return SQLiteRow
	case C.SQLITE_DONE:
		return SQLiteDone
	}

	return ErrGeneric
}
