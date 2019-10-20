package main

import (
	"bot/journal"
	"bot/sqlite"
	"bot/telegrambotapi"
	"bytes"
	"encoding/json"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	dbQueryTimeoutMS = 5000

	posterQuery = `
SELECT poster
  FROM movie_detail
 WHERE title = ?1;
`
)

var (
	dbConn     *sqlite.Conn
	posterStmt *sqlite.Stmt
)

// bot общается по Telegram Bot API с пользователями Telegram.
// IPAddress и port используются при запуске Webhook'а для получения
// сообщений.
func bot(cfg *botConfig, dbName string) {
	goID := "[go bot]:"
	journal.Info(goID, " started")

	// Установка соединения с БД и её настройка.
	dbConn, err := sqlite.NewConn(dbName)
	if err != nil {
		journal.Fatal(err)
		return
	}
	// defer dbConn.Close()
	journal.Info(goID, " connected to database "+dbName)

	err = dbConn.SetBusyTimeout(dbQueryTimeoutMS)
	if err != nil {
		journal.Error(goID, " ", err)
		return
	}
	journal.Trace(goID, " set database connection busy timeout to ", dbQueryTimeoutMS, " ms")

	// Подготавливаем запросы.
	posterStmt, err = dbConn.Prepare(posterQuery)
	if err != nil {
		journal.Fatal(goID, " ", err)
		return
	}
	// defer posterStmt.Close()
	journal.Trace(goID, " poster query prepared")

	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}
	client := telegrambotapi.NewClient(cfg.Token, cfg.BotAPIAddr, httpClient)

	// Установка Webhook'а.
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

func telegramHandler(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		journal.Error(err)
		return
	}

	journal.Info(string(body))

	var update telegrambotapi.Update
	err = json.Unmarshal(body, &update)
	if err != nil {
		journal.Error(err)
		return
	}

	var poster []byte
	err = posterStmt.QueryRow(update.Message.Text).Scan(&poster)
	if err != nil {
		journal.Error(err)
		return
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

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
	fw, err = mw.CreateFormFile("photo", "image.jpg")
	_, err = fw.Write(poster)
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
