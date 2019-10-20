package telegrambotapi

import "encoding/json"

// telegramResponse - это общий вид любого ответа, который возвращает
// Telegram Bot API. Само сообщение хранится в Result, если OK равен true.
type telegramResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

// User - это пользователь Telegram или бот.
// https://core.telegram.org/bots/api#user
type User struct {
	ID        int    `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	UserName  string `json:"username"`
	LangCode  string `json:"language_code"`
}

// WebhookInfo - это информация о Webhook'е.
// https://core.telegram.org/bots/api#webhookinfo
type WebhookInfo struct {
	URL                  string   `json:"url"`
	HasCustomCertificate bool     `json:"has_custom_certificate"`
	PendingUpdateCount   int      `json:"pending_update_count"`
	LastErrorDate        int      `json:"last_error_date"`
	LastErrorMessage     string   `json:"last_error_message"`
	MaxConnections       int      `json:"max_connections"`
	AllowedUpdates       []string `json:"allowed_updates"`
}

// Chat - Telegram чат.
// https://core.telegram.org/bots/api#chat
// TODO: добавить остальные параметры.
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// Message - сообщение Telegram.
// https://core.telegram.org/bots/api#message
// TODO: добавить остальные параметры.
type Message struct {
	ID   int    `json:"message_id"`
	From User   `json:"from"`
	Date int    `json:"date"`
	Chat Chat   `json:"chat"`
	Text string `json:"text"`
}

// Update - новое сообщение от Telegram.
// https://core.telegram.org/bots/api#update
// TODO: добавить остальные параметры.
type Update struct {
	ID      int     `json:"update_id"`
	Message Message `json:"message"`
}
