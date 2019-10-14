package telegrambotapi

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
