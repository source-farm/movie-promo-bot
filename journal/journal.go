// Пакет journal используется для логирования сообщений.
package journal

import (
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

func init() {
	go func() {
		for msgInfo := range msgQueue {
			logMsg := makeLogMsg(msgInfo)
			logger.Print(logMsg)
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

type logMsgInfo struct {
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
	// Уровень логирования по-умолчанию.
	defaultLevel = LevInfo

	// Текущий уровень логирования.
	curLevel = defaultLevel
	clMu     sync.RWMutex

	// Правила замены для логируемых сообщений.
	replaceRule map[string]string = map[string]string{}
	rrMu        sync.RWMutex

	// TODO: os.Stdout заменить на файл.
	logger   = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds)
	msgQueue = make(chan logMsgInfo, 128)
)

// Fatal логирует сообщение в args и заканчивает выполнение приложения.
func Fatal(args ...interface{}) {
	_, file, line, ok := runtime.Caller(1)
	location := ""
	if ok {
		location = path.Base(file) + ":" + strconv.Itoa(line)
	}
	logMsg := makeLogMsg(logMsgInfo{level: LevFatal, location: location, args: args})
	logger.Fatal(logMsg)
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
	clMu.Lock()
	defer clMu.Unlock()
	curLevel = level
}

// Replace настраивает журнал на замену любого появления строки old в логирумых
// сообщениях на строку new.
func Replace(old, new string) {
	if old == new {
		return
	}

	rrMu.Lock()
	defer rrMu.Unlock()
	replaceRule[old] = new
}

// addToQueue добавляет сообщение в очедерь логирования.
func addToQueue(level Level, args ...interface{}) {
	clMu.RLock()
	defer clMu.RUnlock()
	if curLevel >= level {
		_, file, line, ok := runtime.Caller(2)
		location := ""
		if ok {
			location = path.Base(file) + ":" + strconv.Itoa(line)
		}
		select {
		case msgQueue <- logMsgInfo{level: level, location: location, args: args}:
		default:
		}
	}
}

// makeLogMsg формирует готовое к выводу сообщение лога.
func makeLogMsg(msgInfo logMsgInfo) string {
	fullMsg := []interface{}{msgInfo.level, " ", msgInfo.location, ": "}
	fullMsg = append(fullMsg, msgInfo.args...)
	logMsg := fmt.Sprint(fullMsg...)
	rrMu.RLock()
	for old, new := range replaceRule {
		logMsg = strings.Replace(logMsg, old, new, -1)
	}
	rrMu.RUnlock()
	return logMsg
}
