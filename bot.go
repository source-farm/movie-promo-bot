package main

import (
	"bot/journal"
	"bot/telegrambotapi"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

// bot общается по Telegram Bot API с пользователями Telegram.
// IPAddress и port используются при запуске Webhook'а для получения
// сообщений.
func bot(cfg *botConfig, dbName string) {
	goID := "[go bot]:"
	journal.Info(goID, " started")

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
	webhookURL := "https://" + net.JoinHostPort(cfg.WebhookAddr, strconv.Itoa(cfg.WebhookPort)) + "/bot" + cfg.Token
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
	http.HandleFunc("/"+cfg.Token, telegramHandler)
	err = http.ListenAndServeTLS(":"+strconv.Itoa(cfg.WebhookPort), cfg.PublicCert, cfg.PrivateKey, nil)
	if err != nil {
		journal.Fatal(goID, " ", err)
	}
}

func telegramHandler(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	if err == nil {
		journal.Info(string(body))
	}
}
