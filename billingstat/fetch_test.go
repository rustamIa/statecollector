package billingstat

import (
	"fmt"
	"io"
	"log/slog"
	"main/config"
	"main/internal/fileutil"
	"testing"
)

// no-op-логгер для тестов
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// минимальная конфигурация для Get
func makeCfg(fileName string) *config.CfgApp {
	return &config.CfgApp{
		FileBillingState: fileName,
	}
}

// Удобный принтер результата: все 6 полей в строку
func BillingDataToString(data BillingData) string {
	return fmt.Sprintf("%t;%t;%t;%t;%t;%t",
		data.CreateCustomer, data.Purchase, data.Payout,
		data.Recurring, data.FraudControl, data.CheckoutPage)
}

// Mock FileOpener для тестов
func mockFileOpener(filename string) ([]byte, error) {
	// Мокирование содержимого файла
	switch filename {
	case "valid_file":
		return []byte("101010"), nil // Пример действительных данных
	case "empty_file":
		return []byte(""), nil // Пустой файл
	default:
		return nil, fmt.Errorf("file not found")
	}
}

// Тестируем функцию Get
func TestGet(t *testing.T) {
	// Мокируем функцию fileutil.FileOpener
	origOpen := fileutil.FileOpener
	defer func() { fileutil.FileOpener = origOpen }()
	fileutil.FileOpener = mockFileOpener

	tests := []struct {
		name       string
		fileName   string
		wantResult BillingData
		wantErr    bool
	}{
		{
			name:     "valid file",
			fileName: "valid_file",
			wantResult: BillingData{
				CreateCustomer: false,
				Purchase:       true,
				Payout:         false,
				Recurring:      true,
				FraudControl:   false,
				CheckoutPage:   true,
			},
			wantErr: false,
		},
		{
			name:     "empty file",
			fileName: "empty_file",
			wantResult: BillingData{
				CreateCustomer: false,
				Purchase:       false,
				Payout:         false,
				Recurring:      false,
				FraudControl:   false,
				CheckoutPage:   false,
			},
			wantErr: true,
		},
		{
			name:     "file not found",
			fileName: "not_existing_file",
			wantResult: BillingData{
				CreateCustomer: false,
				Purchase:       false,
				Payout:         false,
				Recurring:      false,
				FraudControl:   false,
				CheckoutPage:   false,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := makeCfg(tt.fileName)
			logger := testLogger()

			got, err := Fetch(logger, cfg)

			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.wantResult {
				t.Errorf("Get() = %+v, want %+v", BillingDataToString(got), BillingDataToString(tt.wantResult))
			}
		})
	}
}

// Пример теста для функции getStateDec
func TestGetStateDec(t *testing.T) {
	tests := []struct {
		line    string
		want    uint8
		wantErr bool
	}{
		{
			line:    "101010", // 2^5 + 2^3 + 2^1
			want:    42,
			wantErr: false,
		},
		{
			line:    "111000", // 2^5 + 2^4 + 2^3
			want:    56,
			wantErr: false,
		},
		{
			line:    "000000", // все биты нули
			want:    0,
			wantErr: false,
		},
		{
			line:    "111111", // все биты единицы
			want:    63,
			wantErr: false,
		},
		{
			line:    "w111111", // bad string
			want:    63,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got, err := getStateDec([]byte(tt.line))
			if err != nil {
				if !tt.wantErr {
					t.Errorf("getStateDec().err = %d, want_err %v", err, tt.wantErr)
				}
			} else if got != tt.want {
				t.Errorf("getStateDec() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetStateDec_one(t *testing.T) {
	got, _ := getStateDec([]byte("101010"))
	fmt.Println("got:", got) // Для отладки
	if got != 42 {
		t.Errorf("getStateDec() = %d, want %d", got, 42)
	}
}
