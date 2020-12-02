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
	"time"
)

func init() {
	go func() {
		for {
			select {
			case msgInfo := <-msgQueue:
				logMsg := makeLogMsg(msgInfo)
				logger.Print(logMsg)

			case <-stop:
				for {
					select {
					case msgInfo := <-msgQueue:
						logMsg := makeLogMsg(msgInfo)
						logger.Print(logMsg)

					default:
						close(msgQueue)
						return
					}
				}
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

type logMsgInfo struct {
	time     time.Time
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
	replaceRule = map[string]string{}
	rrMu        sync.Mutex

	// Сообщения направляются на стандартный вывод, т.к. сохранением логов
	// будет заниматься systemd.
	logger   = log.New(os.Stdout, "", 0)
	msgQueue = make(chan logMsgInfo, 128)
	stop     = make(chan struct{})
)

// Fatal логирует сообщение в args и заканчивает выполнение приложения.
func Fatal(args ...interface{}) {
	_, file, line, ok := runtime.Caller(1)
	location := ""
	if ok {
		location = path.Base(file) + ":" + strconv.Itoa(line)
	}
	logMsg := makeLogMsg(logMsgInfo{time: time.Now(), level: LevFatal, location: location, args: args})
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

// Stop выводит оставшиеся сообщения и останавливает логирование.
// Вызов какой-либо другой функции после этого может привести к панике.
func Stop() {
	stop <- struct{}{}
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
		case msgQueue <- logMsgInfo{time: time.Now(), level: level, location: location, args: args}:
		default:
		}
	}
}

// makeLogMsg формирует готовое к выводу сообщение лога.
func makeLogMsg(msgInfo logMsgInfo) string {
	// Выполняем lock всей функции, т.к. непонятно являются ли time.Format,
	// fmt.Sprint и strings.Replace потокобезопасными.
	rrMu.Lock()
	defer rrMu.Unlock()

	msgTime := msgInfo.time.Format("2006/01/02 15:04:05.000000")
	fullMsg := []interface{}{msgTime, " ", msgInfo.level, " ", msgInfo.location, ": "}
	fullMsg = append(fullMsg, msgInfo.args...)
	logMsg := fmt.Sprint(fullMsg...)
	for old, new := range replaceRule {
		logMsg = strings.Replace(logMsg, old, new, -1)
	}

	return logMsg
}
