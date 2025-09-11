package voicedata

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"main/config"
	"main/internal/fileutil"
	m "main/internal/model"
	"main/internal/validateStruct"
)

// no-op-логгер
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// минимальная конфигурация для Get
func makeCfg() *config.CfgApp {
	return &config.CfgApp{
		FileVoiceCall:     "voice.data",
		QuantVoiceDataCol: 8,
	}
}

// удобный принтер результата: все 8 полей в строку
func VoiceCallSliceToString(data []m.VoiceCallData) string {
	var b strings.Builder
	for i, d := range data {
		// %v для float32 печатает "0.86", "0.67" и т.п.
		fmt.Fprintf(&b, "%s;%s;%s;%s;%v;%d;%d;%d",
			d.Country, d.Bandwidth, d.ResponseTime, d.Provider,
			d.ConnectionStability, d.TTFB, d.VoicePurity, d.MedianOfCallsTime,
		)
		if i < len(data)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// -----------------------------------------------------------------------------
// Базовый пример

func TestGet_SampleFile_easy(t *testing.T) {
	// мок fileutil.FileOpener
	origOpen := fileutil.FileOpener
	defer func() { fileutil.FileOpener = origOpen }()

	// мок checker (делаем такой же, как продовый, чтобы не ловить паники на коротких строках)
	//origChk := columnsChecker
	//defer func() { columnsChecker = origChk }()
	//columnsChecker = func(cols []string, want int) bool { return len(cols) == want }

	const sample = ` BL;58;930;E-Voic;0.65;738;83;52
AT40673Transparentalls;0.62;581;38;10
BG;40;609;E-Voice;0.86;160;36;5
DK;11;743;JustPhone;0.67;82;74;41`

	const wantStr = `BG;40;609;E-Voice;0.86;160;36;5
DK;11;743;JustPhone;0.67;82;74;41`

	fileutil.FileOpener = func(_ string) ([]byte, error) {
		return []byte(sample), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	got, err := Fetch(ctx, testLogger(), makeCfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := VoiceCallSliceToString(got)
	if out != wantStr {
		t.Fatalf("mismatch.\nwant:\n%s\n\ngot:\n%s", wantStr, out)
	}
}

// -----------------------------------------------------------------------------
// Основные сценарии (table-driven)
/*
Считываем Voice.data file
проверка строк на соответствие:
	4. Каждая строка должна содержать 8 полей (alpha-2 код страны, текущая
	нагрузка в процентах, среднее время ответа, провайдер, стабильность
	соединения, TTFB, чистота связи, медиана длительности звонка). Строки
	содержащие отличное количество полей не должны попадать в результат
	работы функции. Проверить количество элементов в срезе можно с
	помощью функции len()
	5. Некоторые строки могут быть повреждены, их нужно пропускать и не
	записывать в результат выполнения функции
	6. В результат допускаются только страны прошедшие проверку на
	существование по alpha-2 коду.
	7. В результат допускаются только корректные провайдеры. Допустимые
	провайдеры: TransparentCalls, E-Voice, JustPhone. Все некорректные
	провайдеры нужно пропускать и не добавлять в результат работы
	функции
	8. Строки в которых меньше 8-х полей данных не допускаются
	9. Все целочисленные данные должны быть приведены к типу int
	10.Все числа с плавающей точкой должны быть приведены к типу float32
*/

func TestGet_TableDriven(t *testing.T) {
	origOpen := fileutil.FileOpener
	defer func() { fileutil.FileOpener = origOpen }()

	//origChk := columnsChecker
	//defer func() { columnsChecker = origChk }()
	//columnsChecker = func(cols []string, want int) bool { return len(cols) == want }

	type tc struct {
		name     string
		sample   string
		wantRows int
	}

	tests := []tc{
		{
			name: "all valid lines",
			sample: `RU;86;297;TransparentCalls;0.9;120;80;30
US;64;1923;E-Voice;0.75;200;65;45
GB;88;1892;JustPhone;0.6;150;70;50`,
			wantRows: 3,
		},
		{
			name: "wrong providers are skipped",
			sample: `RU;86;297;Foo;0.9;120;80;30
US;64;1923;E-Voice;0.75;200;65;45
GB;88;1892;Bar;0.6;150;70;50`,
			wantRows: 1, // только US
		},
		{
			name: "invalid alpha-2 codes are skipped",
			sample: `XX;40;100;E-Voice;0.8;100;90;30
USA;50;200;JustPhone;0.7;120;80;40
BL;57;267;TransparentCalls;0.95;90;88;60`,
			wantRows: 1, // только BL
		},
		{
			name: "bandwidth out of range (num0to100) skipped",
			sample: `RU;101;297;TransparentCalls;0.9;120;80;30
US;100;1923;E-Voice;0.75;200;65;45
GB;-1;1892;JustPhone;0.6;150;70;50
GB;50;1892;JustPhone;0.6;150;70;50
DK;0;743;JustPhone;0.67;82;74;41`,
			// пройдут US (100), GB (50), DK (0) — остальные за бортом
			wantRows: 3,
		},
		{
			name: "invalid numeric parsing causes skip",
			/* ResponseTime = NaN
			// ConnStability parseFloat fail -> skip
			// TTFB parse fail -> skip
			// ok
			*/
			sample: `RU;86;NaN;TransparentCalls;0.9;120;80;30
US;64;1923;E-Voice;badfloat;200;65;45
GB;88;1892;JustPhone;0.6;-1.2;70;50
BL;64;1923;E-Voice;1.1;200;s5;45
DK;0;743;JustPhone;0.67;82;;41
CA;8;1059;E-Voice;0.7;100;70;50`,
			wantRows: 1,
		},
		{
			name: "wrong columns (<8 and >8) are skipped",
			sample: `RU;86;297;TransparentCalls;0.9;120;80;30
CA,86297,TransparentCalls,0.9,120,80,30
DE;77;646;0.8;120;80
BG;19;435;E-Voice;0.85;160;36;5;extra
DK;12;1454;JustPhone;0.7;90;70;40`,
			wantRows: 2, // RU, DK
		},
		{
			name: "mixture of problems",
			// ok
			// bad country
			// bad provider
			// bad bandwidth (не numeric)
			// ok
			sample: `RU;86;297;TransparentCalls;0.9;120;80;30
XX;86;297;TransparentCalls;0.9;120;80;30
US;64;1923;Foo;0.75;200;65;45
CA,86297,TransparentCalls,0.9,120,80,30
GB;8p8;1892;JustPhone;0.6;150;70;50
BL;64;1923;E-Voice;1.1;200;s5;45
DK;0;743;JustPhone;0.67;82;;41
CA;8;1059;E-Voice;0.7;100;70;50`,
			wantRows: 2, // RU, CA
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fileutil.FileOpener = func(_ string) ([]byte, error) {
				// нормализуем комментарии в тестовых данных: отрежем " // ..."
				lines := strings.Split(tt.sample, "\n")
				for i := range lines {
					if idx := strings.Index(lines[i], "//"); idx >= 0 {
						lines[i] = strings.TrimSpace(lines[i][:idx])
					}
				}
				return []byte(strings.Join(lines, "\n")), nil
			}

			// убедимся, что кастомные валидаторы зарегистрированы (init в пакете validate уже сделал это)
			_ = validateStruct.Struct(struct{}{}) // no-op, просто дернуть пакет

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
			defer cancel()

			got, err := Fetch(ctx, testLogger(), makeCfg())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != tt.wantRows {
				t.Fatalf("want %d valid rows, got %d\nrows:\n%s",
					tt.wantRows, len(got), VoiceCallSliceToString(got))
			}
		})
	}
}
