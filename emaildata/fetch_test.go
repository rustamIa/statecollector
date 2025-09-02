package emaildata

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"main/config"
	"main/internal/fileutil"
)

// no-op-логгер для тестов
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// минимальная конфигурация для Get
func makeCfg() *config.CfgApp {
	return &config.CfgApp{
		FileVoiceCall:     "email.data", // Путь к файлу
		QuantEmailDataCol: 3,            // Ожидаем 3 колонки
	}
}

// Удобный принтер результата: строки через `;`
func EmailDataSliceToString(data []EmailData) string {
	var b strings.Builder
	for i, e := range data {
		fmt.Fprintf(&b, "%s;%s;%d", e.Country, e.Provider, e.DeliveryTime)
		if i < len(data)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// -----------------------------------------------------------------------------
// Основной тест

func TestGet_SampleFile_easy(t *testing.T) {
	// Мокируем функцию открытия файла
	origOpen := fileutil.FileOpener
	defer func() { fileutil.FileOpener = origOpen }()

	const sample = `RU;Gmail;23
RU;Yahoo;169
RU;Hotmail;63
RU;MSN;475
RU;Orange;519
RU;Comcast;408
RU;AOL;254
RU;Life;538
RU;RediffMail551
RU;GMX;246`

	const wantStr = `RU;Gmail;23
RU;Yahoo;169
RU;Hotmail;63
RU;MSN;475
RU;Orange;519
RU;Comcast;408
RU;AOL;254
RU;GMX;246`

	fileutil.FileOpener = func(_ string) ([]byte, error) {
		return []byte(sample), nil
	}

	got, err := Fetch(testLogger(), makeCfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := EmailDataSliceToString(got)
	if out != wantStr {
		t.Fatalf("mismatch.\nwant:\n%s\n\ngot:\n%s", wantStr, out)
	}
}

// -----------------------------------------------------------------------------
// Основные сценарии (table-driven)

// Тест с различными случаями
func TestGet_TableDriven(t *testing.T) {
	origOpen := fileutil.FileOpener
	defer func() { fileutil.FileOpener = origOpen }()

	tests := []struct {
		name     string
		sample   string
		wantRows int
	}{
		{
			name: "valid rows",
			sample: `RU;Gmail;23
RU;Yahoo;169
RU;Hotmail;63`,
			wantRows: 3,
		},
		{
			name: "testFromTask",
			sample: `T;Gmail;511
AT;Yahoo274
AT;Hotmail;487`,
			wantRows: 1,
		},
		{
			name: "invalid provider",
			sample: `RU;InvalidProvider;200
US;Gmail;374
GB;Yahoo;489`,
			wantRows: 2, // RU строка пропускается
		},
		{
			name: "less than 3 columns",
			sample: `RU;Gmail
US;Gmail;374
GB;Yahoo;489`,
			wantRows: 2, // RU строка пропускается (не хватает колонок)
		},
		{
			name: "more than 3 columns",
			sample: `RU;Gmail;23;Extra
US;Yahoo;237
GB;Hotmail;539`,
			wantRows: 2, // RU строка пропускается (слишком много колонок)
		},
		{
			name: "invalid numeric data in DeliveryTime",
			sample: `RU;Gmail;abc
US;Gmail;374
GB;Yahoo;489`,
			wantRows: 2, // RU строка пропускается (невалидный DeliveryTime)
		},
		{
			name: "invalid country code",
			sample: `XX;Gmail;123
RU;Yahoo;200
GB;Hotmail;300`,
			wantRows: 2, // XX строка пропускается (невалидный код страны)
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fileutil.FileOpener = func(_ string) ([]byte, error) {
				return []byte(tt.sample), nil
			}

			got, err := Fetch(testLogger(), makeCfg())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Проверка количества строк
			if len(got) != tt.wantRows {
				t.Fatalf("expected %d rows, got %d", tt.wantRows, len(got))
			}
		})
	}
}
