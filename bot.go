package main

import (
	"bot/journal"
	"bot/levenshtein"
	"bot/sqlite"
	"bot/telegrambotapi"
	"bytes"
	"container/heap"
	"encoding/json"
	"errors"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Краткая информация о фильме.
type titleInfo struct {
	id            int64     // Значение поля id в таблице movie_detail.
	titleOriginal string    // Название фильма.
	titleLower    string    // Название фильма в нижнем регистре.
	releaseDate   time.Time // Время выхода фильма в кинотеатрах.
	collectionID  int64     // Разные части одного фильма принадлежат одной колеекции.
	editcost      int       // Стоимость приведения какого-либо фильма к title. Чем меньше, тем лучше.
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

	// Извлечения постера фильма по его id в таблице movie_detail.
	posterQuery = `
SELECT poster
  FROM movie_detail
 WHERE id = ?1;
`

	// Извлечение фильмов выше определённого id.
	titlesQuery = `
   SELECT movie_detail.id, movie_detail.title, movie.released_on, movie.collection_id
     FROM movie_detail
LEFT JOIN movie ON movie_detail.fk_movie_id = movie.id
    WHERE movie_detail.id > ?1
 ORDER BY movie_detail.id;
`

	// Стоимости операций для алгоритма Левенштейна.
	levInsCost = 1   // Вставка символа.
	levDelCost = 7   // Удаление символа.
	levSubCost = 100 // Замена символа.

	// Макс. количество вариантов постеров, которые отправляются в ответ на
	// запрос Telegram клиента.
	maxResultsInResponse = 3
)

var (
	posterStmt *sqlite.Stmt
	titlesStmt *sqlite.Stmt
	// Словарь из всех известных боту фильмов. Индексирование идёт по полю id
	// таблицы movie_detail.
	titles      map[int64]titleInfo = map[int64]titleInfo{}
	tlgrmClient *telegrambotapi.Client
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
	err = loadTitles(titles, titlesStmt)
	if err != nil {
		journal.Fatal(err)
	}
	journal.Info(goID, " movie titles loading end")

	// Установка Webhook'а.
	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}
	tlgrmClient = telegrambotapi.NewClient(cfg.Token, cfg.BotAPIAddr, httpClient)
	webhookInfo, err := tlgrmClient.GetWebhookInfo()
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

		err = tlgrmClient.SetWebhook(webhookURL, cert)
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
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		journal.Error(err)
		http.Error(w, "Error while reading request", http.StatusInternalServerError)
		return
	}

	var update telegrambotapi.Update
	err = json.Unmarshal(body, &update)
	if err != nil {
		journal.Error(err)
		http.Error(w, "Cannot unmarshal update", http.StatusInternalServerError)
		return
	}

	switch {
	// Пришло новое сообщение.
	case update.Message.ID != 0:
		sendPhoto, contentType, err := makeSendPhoto(update.Message.Text, update.Message.Chat.ID)
		if err != nil {
			journal.Error(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, err = w.Write(sendPhoto)
		if err != nil {
			journal.Error(err)
			return
		}

	// Пользователь нажал на кнопку ранее отправленного сообщения с inline клавиатурой.
	case update.CallbackQuery.ID != "":
		editMessageMedia, contentType, err := makeEditMessageMedia(&update.CallbackQuery)
		if err != nil {
			journal.Error(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, err = w.Write(editMessageMedia)
		if err != nil {
			journal.Error(err)
			return
		}

		// При нажатии какой-либо кнопки inline клавиатуры необходимо вызывать
		// метод AnswerCallbackQuery Telegram Bot API, чтобы исчез белый круг
		// прогресса на кнопке.
		err = tlgrmClient.AnswerCallbackQuery(update.CallbackQuery.ID)
		if err != nil {
			journal.Error(err)
			return
		}
	}
}

// Загрузка из БД названий фильмов в titles. Повторный вызов функции
// добавляет к titles новые фильмы, а не извлекает всё заново.
func loadTitles(titles map[int64]titleInfo, titlesQuery *sqlite.Stmt) error {
	// В слайсе titles фильмы должны идти по возрастанию поля id.
	// Находим макс. id, чтобы запросить у БД только новые фильмы.
	maxID := int64(0)
	for id := range titles {
		if id > maxID {
			maxID = id
		}
	}

	rows, err := titlesStmt.Query(maxID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, collectionID int64
		var title, releaseDateStr string
		err = rows.Scan(&id, &title, &releaseDateStr, &collectionID)
		if err != nil {
			return err
		}
		releaseDate, err := time.Parse("2006-01-02", releaseDateStr)
		if err != nil {
			journal.Error(err)
			releaseDate = time.Time{}
		}
		t := titleInfo{
			id:            id,
			titleOriginal: title,
			titleLower:    strings.ToLower(title), // Используется в getBestMatchTitles.
			releaseDate:   releaseDate,
			collectionID:  collectionID,
		}
		titles[id] = t
	}
	if rows.Err() != nil {
		return err
	}

	return nil
}

// makeSendPhoto возвращает сообщение sendPhoto из Telegram Bot API:
// https://core.telegram.org/bots/api#sendphoto
// Параметр типа string после сообщения - это значение заголовка Content-Type.
func makeSendPhoto(userInput string, charID int64) ([]byte, string, error) {
	bestMatchTitles := getBestMatchTitles(userInput, titles)
	if len(bestMatchTitles) == 0 {
		return nil, "", errors.New("No match in movies database")
	}

	var poster []byte
	// TODO: posterStmt.QueryRow не является потокобезопасным.
	err := posterStmt.QueryRow(bestMatchTitles[0].id).Scan(&poster)
	if err != nil {
		return nil, "", errors.New("Cannot fetch poster from database")
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Параметр method.
	fw, err := mw.CreateFormField("method")
	if err != nil {
		return nil, "", err
	}
	_, err = fw.Write([]byte("sendPhoto"))
	if err != nil {
		return nil, "", err
	}

	// Параметр chat_id.
	fw, err = mw.CreateFormField("chat_id")
	if err != nil {
		return nil, "", err
	}
	_, err = fw.Write([]byte(strconv.FormatInt(charID, 10)))
	if err != nil {
		return nil, "", err
	}

	// Параметр photo.
	fw, err = mw.CreateFormFile("photo", "image") // Вместо "image" может быть любое другое название.
	_, err = fw.Write(poster)
	if err != nil {
		return nil, "", err
	}

	// Параметр caption.
	fw, err = mw.CreateFormField("caption")
	if err != nil {
		return nil, "", err
	}
	posterCaption := bestMatchTitles[0].titleOriginal
	if !bestMatchTitles[0].releaseDate.IsZero() {
		posterCaption += " (" + strconv.Itoa(bestMatchTitles[0].releaseDate.Year()) + ")"
	}
	_, err = fw.Write([]byte(posterCaption))
	if err != nil {
		return nil, "", err
	}

	// Параметр reply_markup.
	// Формируем клавиатуру из трёх кнопок с названиями "- 1 -",  "2" и т.д. до maxResultsInResponse.
	// Номера кнопок соответствуют фильмам из bestMatchTitles.
	fw, err = mw.CreateFormField("reply_markup")
	if err != nil {
		return nil, "", err
	}
	keyboard := telegrambotapi.InlineKeyboardMarkup{InlineKeyboard: [][]telegrambotapi.InlineKeyboardButton{}}
	buttons := []telegrambotapi.InlineKeyboardButton{}
	for i := range bestMatchTitles {
		buttonText := strconv.Itoa(i + 1)
		if i == 0 {
			buttonText = "- " + strconv.Itoa(i+1) + " -"
		}
		buttons = append(buttons, telegrambotapi.InlineKeyboardButton{
			Text: buttonText,
			// CallbackData - это данные, которые получит бот обратно, когда
			// пользователь нажмёт на соответствующую кнопку. Здесь мы
			// устанавливаем её равной id фильма в таблице movie_detail.
			CallbackData: strconv.FormatInt(bestMatchTitles[i].id, 10),
		})
		if (i + 1) >= maxResultsInResponse {
			break
		}
	}
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, buttons)
	keyboardJSONed, err := json.Marshal(keyboard)
	if err != nil {
		return nil, "", err
	}
	_, err = fw.Write(keyboardJSONed)
	if err != nil {
		return nil, "", err
	}

	mw.Close()

	return buf.Bytes(), mw.FormDataContentType(), nil
}

// makeEditMessageMedia формирует сообщение, которое должно быть выслано в
// ответ на нажатие пользователем какой-либо кнопки inline клавиатуры.  Второй
// возвращаемый параметр типа string - это значения заголовка Content-Type.
// https://core.telegram.org/bots/api#editmessagemedia
func makeEditMessageMedia(callbackQuery *telegrambotapi.CallbackQuery) ([]byte, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Параметр method.
	fw, err := mw.CreateFormField("method")
	if err != nil {
		return nil, "", err
	}
	_, err = fw.Write([]byte("editMessageMedia"))
	if err != nil {
		return nil, "", err
	}

	// Параметр chat_id.
	fw, err = mw.CreateFormField("chat_id")
	if err != nil {
		return nil, "", err
	}
	_, err = fw.Write([]byte(strconv.FormatInt(callbackQuery.Message.Chat.ID, 10)))
	if err != nil {
		return nil, "", err
	}

	// Параметр message_id.
	fw, err = mw.CreateFormField("message_id")
	if err != nil {
		return nil, "", err
	}
	_, err = fw.Write([]byte(strconv.Itoa(callbackQuery.Message.ID)))
	if err != nil {
		return nil, "", err
	}

	// Параметр media.
	fw, err = mw.CreateFormField("media")
	if err != nil {
		return nil, "", err
	}

	// Каждая кнопка inline клавиатуры должна показывать постер при нажатии на
	// неё. Этот постер можно получить из таблицы movie_detail по ID, который
	// хранится в callbackQuery.Data. См. также в функции makeSendPhoto место,
	// где создаётся клавиатура.
	movieID, err := strconv.ParseInt(callbackQuery.Data, 10, 64)
	if err != nil {
		return nil, "", err
	}
	title, ok := titles[movieID]
	if !ok {
		return nil, "", errors.New("Movie not found")
	}

	photoFieldName := "photo"
	inputMediaPhoto := telegrambotapi.InputMediaPhoto{
		Type:    "photo",
		Media:   "attach://" + photoFieldName,
		Caption: title.titleOriginal,
	}
	if !title.releaseDate.IsZero() {
		inputMediaPhoto.Caption += " (" + strconv.Itoa(title.releaseDate.Year()) + ")"
	}
	inputMediaPhotoJSONed, err := json.Marshal(inputMediaPhoto)
	if err != nil {
		return nil, "", err
	}
	_, err = fw.Write(inputMediaPhotoJSONed)
	if err != nil {
		return nil, "", err
	}

	var poster []byte
	// TODO: posterStmt.QueryRow не является потокобезопасным.
	err = posterStmt.QueryRow(movieID).Scan(&poster)
	if err != nil {
		return nil, "", errors.New("Cannot fetch poster from database")
	}
	// Параметр photo.
	fw, err = mw.CreateFormFile(photoFieldName, "image") // Вместо "image" может быть любое другое название.
	_, err = fw.Write(poster)
	if err != nil {
		return nil, "", err
	}

	// Параметр reply_markup.
	// Создаём новую клавиатуру на основе старой. По сути, клавиатура остаётся
	// та же самая, только активной становится нажатая кнопка, т.е. нажатая
	// кнопка выделяется по обеим сторонам знаком "-".
	oldKeyboard := callbackQuery.Message.ReplyMarkup.InlineKeyboard
	if len(oldKeyboard) == 0 {
		return nil, "", errors.New("Empty keyboard")
	}
	fw, err = mw.CreateFormField("reply_markup")
	if err != nil {
		return nil, "", err
	}
	newButtons := []telegrambotapi.InlineKeyboardButton{}
	for i, button := range oldKeyboard[0] { // У нас клавиатура состоит только из одного ряда кнопок.
		buttonText := strconv.Itoa(i + 1)
		if button.CallbackData == callbackQuery.Data {
			buttonText = "- " + strconv.Itoa(i+1) + " -"
		}
		newButtons = append(newButtons, telegrambotapi.InlineKeyboardButton{
			Text:         buttonText,
			CallbackData: button.CallbackData,
		})
	}
	newKeyboard := telegrambotapi.InlineKeyboardMarkup{InlineKeyboard: [][]telegrambotapi.InlineKeyboardButton{newButtons}}
	newKeyboardJSONed, err := json.Marshal(newKeyboard)
	if err != nil {
		return nil, "", err
	}
	_, err = fw.Write(newKeyboardJSONed)
	if err != nil {
		return nil, "", err
	}

	mw.Close()

	return buf.Bytes(), mw.FormDataContentType(), nil
}

// Нахождение для фильма title наиболее близких по алгоритму Левенштейна
// фильмов в titles. Наиболее близкие будут находиться в начале возвращаемого
// слайса.
func getBestMatchTitles(titleUserInput string, titles map[int64]titleInfo) []titleInfo {
	titleLower := strings.ToLower(strings.TrimSpace(titleUserInput))
	if titleLower == "" || len(titles) == 0 {
		return nil
	}

	var titlesHeap titleInfoHeap
	heap.Init(&titlesHeap)

	// Находим 10 самых близких к фильму title фильмов по расстоянию Левенштейна.
	for id := range titles {
		levDist := levenshtein.Distance(titleLower, titles[id].titleLower, levInsCost, levDelCost, levSubCost)
		titleInfo := titles[id]
		titleInfo.editcost = levDist
		heap.Push(&titlesHeap, titleInfo)
		if titlesHeap.Len() > 10 {
			heap.Pop(&titlesHeap)
		}
	}

	// titlesLevRanked должен содержать фильмы в порядке возрастания расстояния
	// Левенштейна, т.е. самые близкие к title фильмы находятся в его начале.
	titlesLevRanked := make([]titleInfo, titlesHeap.Len())
	for i := len(titlesLevRanked) - 1; i >= 0; i-- {
		titlesLevRanked[i] = heap.Pop(&titlesHeap).(titleInfo)
	}

	// Если в начале titlesLevRanked содержит фильмы с одинаковыми названиями, то
	// более выше ставим более позднее снятый фильм. Примером такого фильма
	// является Lion King, который был снят в 1994 и 2019, т.е. выше в списке
	// должен быть фильм 2019 года.
	for i := 1; i < len(titlesLevRanked); i++ {
		if titlesLevRanked[i].titleLower != titlesLevRanked[0].titleLower {
			break
		}
		for j := i; j > 0; j-- {
			if titlesLevRanked[j].releaseDate.After(titlesLevRanked[j-1].releaseDate) {
				titlesLevRanked[j-1], titlesLevRanked[j] = titlesLevRanked[j], titlesLevRanked[j-1]
			} else {
				break
			}
		}
	}

	if len(titlesLevRanked) <= 3 {
		return titlesLevRanked
	}

	//- Формируем окончательный список фильмов.
	//- Сначала должен идти фильм, который по расстоянию Левенштейна оказался на
	//- первом месте. После него должны идти другие части этого фильма,
	//- упорядоченные по дате релиза.
	titlesRanked := []titleInfo{titlesLevRanked[0]}
	if titlesLevRanked[0].collectionID != 0 {
		for _, title := range titlesLevRanked[1:] {
			if title.collectionID == titlesLevRanked[0].collectionID {
				titlesRanked = append(titlesRanked, title)
			}
		}

		if len(titlesRanked) > 1 {
			topTitleOtherParts := titlesRanked[1:]
			sort.Slice(topTitleOtherParts, func(i, j int) bool {
				return topTitleOtherParts[i].releaseDate.Before(topTitleOtherParts[j].releaseDate)
			})
		}
	}

	//- Далее должны идти все части фильма, который оказался на втором месте по
	//- расстоянию Левенштейна, упорядоченные по дате релиза по возрастанию.
	if titlesLevRanked[1].collectionID != titlesLevRanked[0].collectionID || titlesLevRanked[1].collectionID == 0 {
		topRankedCollectionLen := len(titlesRanked)
		titlesRanked = append(titlesRanked, titlesLevRanked[1])
		if titlesLevRanked[1].collectionID != 0 {
			for _, title := range titlesLevRanked[2:] {
				if title.collectionID == titlesLevRanked[1].collectionID {
					titlesRanked = append(titlesRanked, title)
				}
			}
		}

		if len(titlesRanked) > topRankedCollectionLen {
			secondRankedCollection := titlesRanked[topRankedCollectionLen:]
			sort.Slice(secondRankedCollection, func(i, j int) bool {
				return secondRankedCollection[i].releaseDate.Before(secondRankedCollection[j].releaseDate)
			})
		}
	}

	//- В конце идут остальные фильмы.
	for _, title := range titlesLevRanked[2:] {
		if title.collectionID == 0 ||
			title.collectionID != titlesLevRanked[0].collectionID && title.collectionID != titlesLevRanked[1].collectionID {
			titlesRanked = append(titlesRanked, title)
		}
	}

	return titlesRanked
}
