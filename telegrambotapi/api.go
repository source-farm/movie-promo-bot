package telegrambotapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"mime/multipart"
	"net/http"
)

// Client используется для выполнения запросов к Telegram Bot API.
type Client struct {
	token      string
	httpClient *http.Client
	apiBaseURL string
}

// NewClient возвращает новый Telegram Bot API клиент. botAPIAddr может
// содержать порт в конце адреса, который указывается через ":". Если
// httpClient равен nil, то client будет пользоваться http.DefaultClient'ом.
func NewClient(token, botAPIAddr string, httpClient *http.Client) *Client {
	client := Client{
		token:      token,
		httpClient: httpClient,
		apiBaseURL: "https://" + botAPIAddr + "/bot" + token,
	}
	if client.httpClient == nil {
		client.httpClient = http.DefaultClient
	}
	return &client
}

// GetMe реализует метод getMe Telegram Bot API.
// https://core.telegram.org/bots/api#getme
func (c *Client) GetMe() (*User, error) {
	tlgrmResp, err := c.get("/getMe")
	if err != nil {
		return nil, err
	}

	var user User
	err = json.Unmarshal(tlgrmResp.Result, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetWebhookInfo реализует метод getWebhookInfo Telegram Bot API.
// https://core.telegram.org/bots/api#getwebhookinfo
func (c *Client) GetWebhookInfo() (*WebhookInfo, error) {
	tlgrmResp, err := c.get("/getWebhookInfo")
	if err != nil {
		return nil, err
	}

	var webhookInfo WebhookInfo
	err = json.Unmarshal(tlgrmResp.Result, &webhookInfo)
	if err != nil {
		return nil, err
	}
	return &webhookInfo, nil
}

// SetWebhook реализует метод SetWebhook Telegram Bot API.
// https://core.telegram.org/bots/api#setwebhook
// TODO: добавить недостающие параметры.
func (c *Client) SetWebhook(url string, certificate []byte) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Параметр url.
	fw, err := mw.CreateFormField("url")
	if err != nil {
		return err
	}
	_, err = fw.Write([]byte(url))
	if err != nil {
		return err
	}

	// Параметр certificate.
	fw, err = mw.CreateFormFile("certificate", "public.pem")
	_, err = fw.Write(certificate)
	if err != nil {
		return err
	}

	mw.Close()

	resp, err := c.httpClient.Post(c.apiBaseURL+"/setWebhook", mw.FormDataContentType(), &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("telegrambotapi: " + resp.Status)
	}

	return nil
}

// DeleteWebhook реализует метод deleteWebhook Telegram Bot API.
// https://core.telegram.org/bots/api#deletewebhook
func (c *Client) DeleteWebhook() error {
	resp, err := c.httpClient.Post(c.apiBaseURL+"/deleteWebhook", "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("telegrambotapi: " + resp.Status)
	}

	return nil
}

// AnswerCallbackQuery реализует метод AnswerCallbackQuery Telegram Bot API.
// https://core.telegram.org/bots/api#answercallbackquery
// TODO: добавить недостающие параметры.
func (c *Client) AnswerCallbackQuery(callbackQueryID string) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Параметр callback_query_id.
	fw, err := mw.CreateFormField("callback_query_id")
	if err != nil {
		return err
	}
	_, err = fw.Write([]byte(callbackQueryID))
	if err != nil {
		return err
	}

	mw.Close()

	resp, err := c.httpClient.Post(c.apiBaseURL+"/answerCallbackQuery", mw.FormDataContentType(), &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("telegrambotapi: " + resp.Status)
	}

	return nil
}

// get выполняет GET запросы к Telegram Bot API.
func (c *Client) get(path string) (*telegramResponse, error) {
	resp, err := c.httpClient.Get(c.apiBaseURL + path)
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
	return &tlgrmResp, nil
}
