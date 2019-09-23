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
			if msg.level != LevFatal {
				logger.Print(msg.level, " ", msg.location, ": ", msg.msg)
			} else {
				logger.Fatal(LevFatal, " ", msg.location, ": ", msg.msg)
			}
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
	msg      interface{}
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

// Fatal логирует сообщение msg и заканчивает выполнение приложения.
func Fatal(msg interface{}) {
	_, file, line, ok := runtime.Caller(1)
	location := ""
	if ok {
		location = path.Base(file) + ":" + strconv.Itoa(line)
	}
	logger.Fatal(LevFatal, " ", location, ": ", msg)
}

// Error логирует ошибки.
// Уровень логирования должен быть LevError или выше.
func Error(msg interface{}) {
	addToQueue(LevError, msg)
}

// Info логирует информационные сообщения.
// Уровень логирования должен быть LevInfo или выше.
func Info(msg interface{}) {
	addToQueue(LevInfo, msg)
}

// Trace логирует всё.
// Уровень логирования должен быть LevTrace.
func Trace(msg interface{}) {
	addToQueue(LevTrace, msg)
}

// SetLevel устанавливает уровень логирования.
func SetLevel(level Level) {
	mu.Lock()
	defer mu.Unlock()
	curLevel = level
}

func addToQueue(level Level, msg interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	if curLevel >= level {
		_, file, line, ok := runtime.Caller(2)
		location := ""
		if ok {
			location = path.Base(file) + ":" + strconv.Itoa(line)
		}
		select {
		case msgQueue <- logMsg{level: level, location: location, msg: msg}:
		default:
		}
	}
}
