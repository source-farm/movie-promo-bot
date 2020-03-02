<p align="center">
  <img src="other/logo.png">
</p>
<h1 align="center">MoviePromo</h1>

Данный репозиторий содержит исходный код Telegram бота [MoviePromo](https://t.me/MoviePromoBot). Этот бот показывает постер фильма по его названию. Название можно вводить на английском или на русском. Для английских названий показывается английский постер, для русских - русский. База постеров пополняется ежедневно.

## Реализация
Бот написан на Go. Из стороннего используется только библиотека [SQLite](https://www.sqlite.org) в исходном виде, т.е. в виде C кода. Для работы с SQLite написан свой драйвер - пакет [sqlite](https://github.com/source-farm/movie-promo-bot/tree/master/sqlite). Интерфейс sqlite похож на интерфейс [database/sql](https://golang.org/pkg/database/sql/) из стандартной библиотеки, но он не полностью соответствует database/sql. Всё реализовано по минимуму.  

База постеров пополняется по [The Movie Database API](https://developers.themoviedb.org/3/getting-started/introduction). Взаимодействие с Telegram юзерами идёт по [Telegram Bot API](https://core.telegram.org/bots/api) через [webhook'и](https://core.telegram.org/bots/webhooks).  

В качестве системы управления версиями использовался [fossil](https://fossil-scm.org/home/doc/trunk/www/index.wiki). Для заливки в github проект был экспортирован в формат репозитория git.

## Сборка и запуск
Для сборки бота можно воспользоваться скриптом build.sh в корне проекта. После запуска бот вычитывает настройки из файла config.json, который должен находиться в одной папке с ботом. Пример настроек находится в файле other/config_example.json. В поле "themoviedb_key" надо сохранить ключ, который можно получить после регистрации в [themoviedb.org](https://www.themoviedb.org/), а поле "telegram_token" должно содержать Telegram токен бота. Токен выдаётся при создании бота через [BotFather](https://t.me/BotFather). Поля "public_cert" и "private_key" содержат названия файлов открытого сертификата и закрытого ключа соответственно. Эти файлы нужны для работы Telegram webhook'ов и тоже должны находиться в одной папке с ботом. О том как получить эти файлы можно прочитать в [docs/TelegramWebhook.txt](https://github.com/source-farm/movie-promo-bot/blob/master/docs/TelegramWebhook.txt) или в [официальной документации](https://core.telegram.org/bots/webhooks). В принципе бот можно запустить как обычный запускаемый файл через терминал, но если нужно оформить его как systemd сервис, то за основу можно взять [этот](https://github.com/source-farm/movie-promo-bot/blob/master/other/movie-promo-bot.service) unit файл.
