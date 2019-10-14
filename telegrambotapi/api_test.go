package telegrambotapi

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestGetMe(t *testing.T) {
	token, proxyAddr, err := getTestConfig()
	if err != nil {
		t.Fatal(err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Не проверяем сертификат сервера.
			},
		},
		Timeout: time.Second * 10,
	}

	client := NewClient(token, proxyAddr, httpClient)
	user, err := client.GetMe()
	if err != nil {
		t.Fatal(err)
	}
	if !user.IsBot {
		t.Fatal("Not a bot")
	}
}

func getTestConfig() (string, string, error) {
	f, err := os.Open("test_config.json")
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	tesetConfigRaw, err := ioutil.ReadAll(f)
	if err != nil {
		return "", "", err
	}
	type TestConfig struct {
		Token     string `json:"token"`
		ProxyAddr string `json:"proxy_addr"`
		ProxyPort int    `json:"proxy_port"`
	}
	var testConfig TestConfig
	err = json.Unmarshal(tesetConfigRaw, &testConfig)
	if err != nil {
		return "", "", err
	}

	return testConfig.Token, testConfig.ProxyAddr + ":" + strconv.Itoa(testConfig.ProxyPort), nil
}
