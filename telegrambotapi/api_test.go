package telegrambotapi

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"
)

// В документе Тестирование_Telegram_Bot_API.txt можно прочитать о том как
// нужно проводить тестирование.

type testConfig struct {
	Token     string `json:"token"`
	ProxyAddr string `json:"proxy_addr"`
	ProxyPort string `json:"proxy_port"`
}

func TestGetMe(t *testing.T) {
	client, err := getTestClient()
	if err != nil {
		t.Fatal(err)
	}

	user, err := client.GetMe()
	if err != nil {
		t.Fatal(err)
	}
	if !user.IsBot {
		t.Fatal("Not a bot")
	}
}

func TestGetWebhookInfo(t *testing.T) {
	client, err := getTestClient()
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetWebhookInfo()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetWebhook(t *testing.T) {
	client, err := getTestClient()
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Open("public.pem")
	if err != nil {
		t.Fatal(err)
	}
	cert, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := getTestConfig()
	if err != nil {
		t.Fatal(err)
	}

	// 8443 - один из портов, на который может отправлять данные сервера
	// Telegram.  cfg.ProxyAddr находится также и в сертификате public.pem в
	// качестве CN (можно увидеть командой openssl x509 -in public.pem -text).
	// Более подробно можно прочитать в документе TelegramWebhook.txt
	hookURL := "https://" + cfg.ProxyAddr + ":8443/" + cfg.Token
	err = client.SetWebhook(hookURL, cert)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeleteWebhook(t *testing.T) {
	client, err := getTestClient()
	if err != nil {
		t.Fatal(err)
	}

	err = client.DeleteWebhook()
	if err != nil {
		t.Fatal(err)
	}
}

func getTestConfig() (*testConfig, error) {
	f, err := os.Open("test_config.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	testConfigRaw, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var cfg testConfig
	err = json.Unmarshal(testConfigRaw, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func getTestClient() (*Client, error) {
	cfg, err := getTestConfig()
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Не проверяем сертификат сервера.
			},
		},
		Timeout: time.Second * 10,
	}

	client := NewClient(cfg.Token, cfg.ProxyAddr+":"+cfg.ProxyPort, httpClient)
	return client, nil
}
