package telegrambotapi

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
)

// telegramResponse - это общий вид любого ответа, который возвращает
// Telegram Bot API. Само сообщение хранится в Result, если OK равен true.
type telegramResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

// Client используется для выполнения запросов к Telegram Bot API.
type Client struct {
	token      string
	httpClient *http.Client
	apiBaseURL string
}

// NewClient возвращает новый Telegram Bot API клиент. addr может содержать
// порт в конце адреса, который указывается через знак ":". Если httpClient
// равен nil, то client будет пользоваться http.DefaultClient'ом.
func NewClient(token, addr string, httpClient *http.Client) *Client {
	client := Client{
		token:      token,
		httpClient: httpClient,
		apiBaseURL: "https://" + addr + "/bot" + token,
	}
	if client.httpClient == nil {
		client.httpClient = http.DefaultClient
	}
	return &client
}

// GetMe реализует метод getMe Telegram Bot API.
// https://core.telegram.org/bots/api#getme
func (c *Client) GetMe() (*User, error) {
	resp, err := c.httpClient.Get(c.apiBaseURL + "/getMe")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("telegrambotapi: " + resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tlgrmResp telegramResponse
	err = json.Unmarshal(body, &tlgrmResp)
	if err != nil {
		return nil, err
	}
	if !tlgrmResp.OK {
		return nil, errors.New("telegrambotapi: " + tlgrmResp.Description)
	}

	var user User
	err = json.Unmarshal(tlgrmResp.Result, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
