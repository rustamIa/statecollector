package sl

import (
	"log/slog"
)

//функция для логирования ошибок в slog от разных функций, сразу ключи прокидывает в slog

func Err(err error) slog.Attr {
	return slog.Attr{
		Key:   "error",
		Value: slog.StringValue(err.Error()),
	}
}
