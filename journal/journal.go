// Пакет journal используется для логирования сообщений.
package journal

import (
	"log"
	"os"
	"path"
	"runtime"
	"strconv"
	"sync"
)

func init() {
	go func() {
		for msg := range msgQueue {
			fullMsg := []interface{}{msg.level, " ", msg.location, ": "}
			fullMsg = append(fullMsg, msg.args...)
			logger.Print(fullMsg...)
		}
	}()
}

// Level обозначает уровень логирования.
type Level uint8

func (l Level) String() string {
	level := ""

	switch l {
	case LevFatal:
		level = "[FATAL]"

	case LevError:
		level = "[ERROR]"

	case LevInfo:
		level = "[INFO]"

	case LevTrace:
		level = "[TRACE]"
	}

	return level
}

type logMsg struct {
	level    Level
	location string
	args     []interface{}
}

// Уровни логирования.
const (
	LevFatal Level = iota
	LevError
	LevInfo
	LevTrace
)

var (
	defaultLevel = LevInfo
	// TODO: os.Stdout заменить на файл.
	logger   = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds)
	msgQueue = make(chan logMsg, 128)
)

var (
	curLevel = defaultLevel
	mu       sync.RWMutex
)

// Fatal логирует сообщение в args и заканчивает выполнение приложения.
func Fatal(args ...interface{}) {
	_, file, line, ok := runtime.Caller(1)
	location := ""
	if ok {
		location = path.Base(file) + ":" + strconv.Itoa(line)
	}
	fullMsg := []interface{}{LevFatal, " ", location, ": "}
	fullMsg = append(fullMsg, args...)
	logger.Fatal(fullMsg...)
}

// Error логирует ошибки.
// Уровень логирования должен быть LevError или выше.
func Error(args ...interface{}) {
	addToQueue(LevError, args...)
}

// Info логирует информационные сообщения.
// Уровень логирования должен быть LevInfo или выше.
func Info(args ...interface{}) {
	addToQueue(LevInfo, args...)
}

// Trace логирует всё.
// Уровень логирования должен быть LevTrace.
func Trace(args ...interface{}) {
	addToQueue(LevTrace, args...)
}

// SetLevel устанавливает уровень логирования.
func SetLevel(level Level) {
	mu.Lock()
	defer mu.Unlock()
	curLevel = level
}

func addToQueue(level Level, args ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	if curLevel >= level {
		_, file, line, ok := runtime.Caller(2)
		location := ""
		if ok {
			location = path.Base(file) + ":" + strconv.Itoa(line)
		}
		select {
		case msgQueue <- logMsg{level: level, location: location, args: args}:
		default:
		}
	}
}
