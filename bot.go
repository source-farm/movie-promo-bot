package main

import (
	"bot/journal"
	"bot/levenshtein"
	"bot/sqlite"
	"bot/telegrambotapi"
	"bytes"
	"container/heap"
	"encoding/json"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Краткая информация о названии фильма.
type titleInfo struct {
	id            int64  // Значение поля id в таблице movie_detail.
	titleOriginal string // Название фильма.
	titleLower    string // Название фильма в нижнем регистре.
	editcost      int    // Стоимость приведения какого-либо фильма к title. Чем меньше, тем лучше.
}

// Max-куча из значений типа titleInfo.
type titleInfoHeap []titleInfo

func (h titleInfoHeap) Len() int           { return len(h) }
func (h titleInfoHeap) Less(i, j int) bool { return h[i].editcost > h[j].editcost }
func (h titleInfoHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *titleInfoHeap) Push(x interface{}) {
	*h = append(*h, x.(titleInfo))
}

func (h *titleInfoHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

const (
	// Таймаут выполнения запроса к БД.
	dbQueryTimeoutMS = 10000

	// Извлечения постера и даты выхода фильма по id фильма в таблице movie_detail.
	posterQuery = `
   SELECT movie_detail.poster, movie.released_on
     FROM movie_detail
LEFT JOIN movie ON movie_detail.fk_movie_id = movie.id
    WHERE movie_detail.id = ?1;
`

	// Получение названий фильмов.
	titlesQuery = `
  SELECT id, title
    FROM movie_detail
   WHERE id > ?1
ORDER BY id;
`

	// Стоимости операций для алгоритма Левенштейна.
	levInsCost = 1   // Вставка символа.
	levDelCost = 5   // Удаление символа.
	levSubCost = 100 // Замена символа.
)

var (
	posterStmt  *sqlite.Stmt
	titlesStmt  *sqlite.Stmt
	movieTitles []titleInfo
)

// bot общается по Telegram Bot API с пользователями Telegram.
func bot(cfg *botConfig, dbName string) {
	goID := "[go bot]:"
	journal.Info(goID, " started")

	// Установка соединения с БД и её настройка.
	dbConn, err := sqlite.NewConn(dbName)
	if err != nil {
		journal.Fatal(err)
	}
	defer dbConn.Close()
	journal.Info(goID, " connected to database "+dbName)

	err = dbConn.SetBusyTimeout(dbQueryTimeoutMS)
	if err != nil {
		journal.Error(goID, " ", err)
	}
	journal.Trace(goID, " set database busy timeout to ", dbQueryTimeoutMS, " ms")

	// Подготавливаем запросы.
	posterStmt, err = dbConn.Prepare(posterQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
	}
	defer posterStmt.Close()
	journal.Trace(goID, " poster query prepared")

	titlesStmt, err = dbConn.Prepare(titlesQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
	}
	defer titlesStmt.Close()
	journal.Trace(goID, " titles query prepared")

	// Загружаем названия фильмов из БД.
	journal.Info(goID, " loading movie titles from database")
	err = loadTitles(&movieTitles, titlesStmt)
	if err != nil {
		journal.Fatal(err)
	}
	journal.Info(goID, " movie titles loading end")

	// Установка Webhook'а.
	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}
	client := telegrambotapi.NewClient(cfg.Token, cfg.BotAPIAddr, httpClient)
	webhookInfo, err := client.GetWebhookInfo()
	if err != nil {
		journal.Fatal(goID, " ", err)
	}
	// cfg.WebhookAddr и cfg.PublicCert взаимосвязаны. Подробнее можно
	// прочитать в TelegramWebhook.txt.
	webhookPath := "/" + cfg.Token
	webhookURL := "https://" + net.JoinHostPort(cfg.WebhookAddr, strconv.Itoa(cfg.WebhookPort)) + webhookPath
	if !(webhookInfo.URL == webhookURL && webhookInfo.HasCustomCertificate) {
		f, err := os.Open(cfg.PublicCert)
		if err != nil {
			journal.Fatal(goID, " ", err)
		}
		defer f.Close()
		cert, err := ioutil.ReadAll(f)
		if err != nil {
			journal.Fatal(goID, " ", err)
		}

		err = client.SetWebhook(webhookURL, cert)
		if err != nil {
			journal.Fatal(goID, " ", err)
		}
		journal.Info(goID, " webhook set OK")
	} else {
		journal.Info(goID, " webhook is already set")
	}

	// Запускаем обработчик сообщений от Telegram.
	http.HandleFunc(webhookPath, telegramHandler)
	err = http.ListenAndServeTLS(":"+strconv.Itoa(cfg.WebhookPort), cfg.PublicCert, cfg.PrivateKey, nil)
	if err != nil {
		journal.Fatal(goID, " ", err)
	}
}

// Обработчик событий от Telegram.
func telegramHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: дополнить обработку ошибок. Там где нужно отправить серверу, что не так.
	// Сообщить клиенту что-либо адекватное, если фильм не найден.

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		journal.Error(err)
		return
	}

	var update telegrambotapi.Update
	err = json.Unmarshal(body, &update)
	if err != nil {
		journal.Error(err)
		return
	}

	match := getBestMatchTitles(update.Message.Text, movieTitles)
	if len(match) == 0 {
		journal.Info("No match")
		return
	}
	var poster []byte
	var releaseDateStr string
	err = posterStmt.QueryRow(match[0].id).Scan(&poster, &releaseDateStr)
	if err != nil {
		journal.Error(err)
		return
	}
	releaseDate, err := time.Parse("2006-01-02", releaseDateStr)
	if err != nil {
		journal.Error(err)
		releaseDate = time.Time{}
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Используем метод sendPhoto из Telegram Bot API для отправки постера.
	// https://core.telegram.org/bots/api#sendphoto

	// Параметр method.
	fw, err := mw.CreateFormField("method")
	if err != nil {
		journal.Error(err)
		return
	}
	_, err = fw.Write([]byte("sendPhoto"))
	if err != nil {
		journal.Error(err)
		return
	}

	// Параметр chat_id.
	fw, err = mw.CreateFormField("chat_id")
	if err != nil {
		journal.Error(err)
		return
	}
	_, err = fw.Write([]byte(strconv.FormatInt(update.Message.Chat.ID, 10)))
	if err != nil {
		journal.Error(err)
		return
	}

	// Параметр photo.
	fw, err = mw.CreateFormFile("photo", "image")
	_, err = fw.Write(poster)
	if err != nil {
		journal.Error(err)
		return
	}

	// Параметр caption.
	fw, err = mw.CreateFormField("caption")
	if err != nil {
		journal.Error(err)
		return
	}
	posterCaption := match[0].titleOriginal
	if !releaseDate.IsZero() {
		posterCaption += " (" + strconv.Itoa(releaseDate.Year()) + ")"
	}
	_, err = fw.Write([]byte(posterCaption))
	if err != nil {
		journal.Error(err)
		return
	}

	mw.Close()

	w.Header().Set("Content-Type", mw.FormDataContentType())
	_, err = w.Write(buf.Bytes())
	if err != nil {
		journal.Error(err)
		return
	}
}

// Загрузка из БД названий фильмов в titles.
func loadTitles(titles *[]titleInfo, titlesQuery *sqlite.Stmt) error {
	// В слайсе titles фильмы должны идти по возрастанию поля id.
	// Находим макс. id, чтобы запросить у БД только новые фильмы.
	maxID := int64(0)
	if len(*titles) != 0 {
		maxID = (*titles)[len(*titles)-1].id
	}

	rows, err := titlesStmt.Query(maxID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var title string
		err = rows.Scan(&id, &title)
		if err != nil {
			return err
		}
		// Переводим фильмы в нижний регистр для улучшения поиска по ним (см.
		// getBestMatchTitles).
		*titles = append(*titles, titleInfo{id: id, titleOriginal: title, titleLower: strings.ToLower(title)})
	}
	if rows.Err() != nil {
		return err
	}

	return nil
}

// Нахождение для фильма title наиболее близких по алгоритму Левенштейна
// фильмов в titles. Наиболее близкие будут находиться в начале возвращаемого
// слайса.
func getBestMatchTitles(title string, titles []titleInfo) []titleInfo {
	source := strings.ToLower(strings.TrimSpace(title))
	if source == "" || len(titles) == 0 {
		return nil
	}

	var titlesHeap titleInfoHeap
	heap.Init(&titlesHeap)

	for i := range titles {
		levDist := levenshtein.Distance(source, titles[i].titleLower, levInsCost, levDelCost, levSubCost)
		titleInfo := titles[i]
		titleInfo.editcost = levDist
		heap.Push(&titlesHeap, titleInfo)
		if titlesHeap.Len() > 5 { // Отбираем 5 самых близких к source фильмов.
			heap.Pop(&titlesHeap)
		}
	}

	// titlesRanked содержит названия фильмов в порядке возрастания расстояния
	// Левенштейна, т.е. самые близкие к title фильмы находятся в начале
	// слайса.
	titlesRanked := make([]titleInfo, titlesHeap.Len())
	for i := len(titlesRanked) - 1; i >= 0; i-- {
		titlesRanked[i] = heap.Pop(&titlesHeap).(titleInfo)
	}

	// Перемещаем в начало списка те фильмы, в названии которых есть текст,
	// по-которому мы искали фильмы (source).  Это делается для того, чтобы
	// разные части одного фильма оказались сгруппированы рядом.
	for i := range titlesRanked {
		if strings.Index(titlesRanked[i].titleLower, source) != -1 {
			for j := i; j > 0; j-- {
				if strings.Index(titlesRanked[j-1].titleLower, source) == -1 {
					titlesRanked[j-1], titlesRanked[j] = titlesRanked[j], titlesRanked[j-1]
				} else {
					break
				}
			}
		}
	}

	return titlesRanked
}
