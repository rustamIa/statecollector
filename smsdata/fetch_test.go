package smsdata

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"main/config"
	"main/internal/fileutil"
	m "main/internal/model"
	"strings"
	"testing"
	"time"
)

// testLogger — no-op-логгер: пишет в io.Discard, чтобы не засорять вывод go test. -io.Discard	Полностью глушит лог-вывод в тестах.
var testLogger = slog.New(
	slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
)

// makeCfg возвращает минимальный конфиг, который нужен smsdata.Get.
// Поле FileSms не используется, потому что fileReader подменяется в каждом тесте.
func makeCfg() *config.CfgApp {
	return &config.CfgApp{
		FileSms:         "sms.data",
		QuantSMSDataCol: 4,
	}
}

// ----------------------------------------------------------------------------
/*
-каждая строка 4 поля: 1: код страны alpha-2; 2:пропуск способность; 3:среднее время ответа ms; 4: название компании
-строки могут быть повреждены:

	-строки в которых менее 4х или более 4х полей - пропустить ( критерий 1)
	-в результат только страны прошедшие проверку по alpha-2 коду ( критерий 2)
	-в результат только провайдеры Topolo, Rond, Kildy ( критерий 3)
	-пропускная  способность канала от 0% до 100% ( критерий 4)
	-среднее время ответа в ms ( критерий 5)
*/
func TestGet_SampleFile_easy(t *testing.T) {
	orig := fileutil.FileOpener
	defer func() { fileutil.FileOpener = orig }() //мокнули функцию открытия файла

	const sample = `U5;41910;Topol
US;36;1576;Rond
GB28495Topolo
F2;9;484;Topolo
BL;68;1594;Kildy
RU;86;297;Rond
GB;8p8;1892;Topolo
CH;32;1s34;Topolo  
CA;855;1059;Rond`
	const goodResult = `US;36;1576;Rond
BL;68;1594;Kildy
RU;86;297;Rond`

	fileutil.FileOpener = func(_ string) ([]byte, error) {
		return []byte(sample), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	got, err := Fetch(ctx, testLogger, makeCfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := SMSDataSliceToString(got)
	if out != goodResult {
		t.Fatalf("sample doesn't match goodResult")
	}
}

func SMSDataSliceToString(data []m.SMSData) string {
	var b strings.Builder
	for i, d := range data {
		fmt.Fprintf(&b, "%s;%s;%s;%s", d.Country, d.Bandwidth, d.ResponseTime, d.Provider)
		if i < len(data)-1 {
			b.WriteByte('\n') // перевод строки между элементами
		}
	}
	return b.String()
}

// -- empty
func TestGet_SampleFile_empty(t *testing.T) {
	orig := fileutil.FileOpener
	defer func() { fileutil.FileOpener = orig }() //мокнули функцию открытия файла

	const sample = ``

	fileutil.FileOpener = func(_ string) ([]byte, error) {
		return []byte(sample), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	got, err := Fetch(ctx, testLogger, makeCfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fmt.Println("got =", got)
	if got == nil {
		t.Fatalf("result is nil")
	}
}

// ---------- основные тесты --------------------------------------------------

func TestGet_TableDriven(t *testing.T) {
	orig := fileutil.FileOpener
	defer func() { fileutil.FileOpener = orig }() // обязательно восстанавливаем!

	tests := []struct {
		name     string
		sample   string
		wantRows int // сколько валидных записей должны получить
	}{
		{
			name: "all valid lines",
			sample: `RU;86;297;Topolo
US;64;1923;Rond
GB;88;1892;Topolo
FR;61;170;Topolo
BL;57;267;Kildy
AT;77;646;Topolo
BG;19;435;Rond
DK;12;1454;Topolo
CA;8;1059;Rond
ES;83;784;Topolo
CH;32;1934;Topolo
TR;68;1629;Rond
PE;17;414;Topolo
NZ;31;1275;Kildy
MC;29;1683;Kildy`,
			wantRows: 15,
		},
		{
			name: "from excercise",
			sample: `U5;41910;Topol
US;36;1576;Rond
GB28495Topolo
F2;9;484;Topolo
BL;68;1594;Kildy`,
			wantRows: 2, // остаются только US и BL
		},
		{
			name: "invalid bandwidth",
			sample: `US;36;1576;Rond
BL;6658;1594;Kildy`,
			wantRows: 1, // остаются только US
		},
		{
			name: "lines with wrong columns (<4, >4) are skipped",
			sample: `RU;86;297;Topolo
RU;12;34                         
US;99;88;Rond;extra              
GB;88;1892;Topolo`,
			wantRows: 2, // остаются только RU1 и GB4
		},
		{
			name: "invalid alpha-2 country codes are skipped",
			sample: `XX;12;34;Rond          
US;64;1923;Rond
USA;88;1892;Topolo    
NZ;31;1275;Kildy`,
			wantRows: 2, // US и NZ
		},
		{
			name: "invalid provider names are skipped",
			sample: `RU;86;297;Foo          
US;64;1923;Rond
GB;88;1892;Bar          
CA;8;1059;Rond`,
			wantRows: 2, // US и CA
		},
		{
			name: "invalid type of fild bandwidth",
			sample: `RU;86;297;Rond
US;64;1923;Rond
GB;8p8;1892;Topolo
CH;32;1934;Topolo
CA;8;1059;Rond`,
			wantRows: 4, // RU, US, CH, CA
		},
		{
			name: "invalid type of fild response time",
			sample: `RU;86;%;Rond
US;64;r923;Rond
GB;88;1892;Topolo
CA;8;1059;Rond`,
			wantRows: 2, // GB, CA
		},
		{
			name: "mixture of all problems",
			/*
			   `RU;86;297;Topolo       // ok
			   XX;86;297;Topolo       // bad country
			   DE;77;646               // bad columns (<4)
			   BG;19;435;Rond          // ok
			   ES;83;784;Foo           // bad provider
			   DK;12;1454;Topolo`, // ok
			*/
			sample: `RU;86;297;Topolo
RU;86;%;Rond  
XX;86;297;Topolo
DE;77;646   
GB;8p8;1892;Topolo
BG;19;435;Rond
ES;83;784;Foo
DK;12;1454;Topolo`,
			wantRows: 3, // RU, BG, DK
		},
	}

	allowedProv := map[string]struct{}{"Topolo": {}, "Rond": {}, "Kildy": {}}

	for _, tt := range tests {
		tt := tt // pin внутри цикла
		t.Run(tt.name, func(t *testing.T) {
			// подменяем «чтение файла»
			fileutil.FileOpener = func(_ string) ([]byte, error) {
				return []byte(tt.sample), nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
			defer cancel()

			got, err := Fetch(ctx, testLogger, makeCfg())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			fmt.Print(got)

			// --- в каждом возвращённом объекте должно быть 4 непустых поля
			for i, r := range got {
				if r.Country == "" || r.Bandwidth == "" || r.ResponseTime == "" || r.Provider == "" {
					t.Errorf("row %d has empty fields: %#v", i, r)
				}
			}

			// --- проверяем, что бракованные строки не прошли
			if len(got) != tt.wantRows {
				t.Fatalf("want %d valid rows, got %d", tt.wantRows, len(got))
			}
			for i, r := range got {
				if len(r.Country) != 2 { // простая alpha-2 проверка
					t.Errorf("row %d: invalid country %q", i, r.Country)
				}
				if _, ok := allowedProv[r.Provider]; !ok {
					t.Errorf("row %d: invalid provider %q", i, r.Provider)
				}
			}
		})
	}
}
