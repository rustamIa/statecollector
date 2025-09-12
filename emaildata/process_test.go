package emaildata

import (
	"context"
	"io"
	"log/slog"
	"main/config"
	m "main/internal/model"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// Простой парсер для теста: берём только корректные строки "CC;Provider;Number".
func parseEmailCSVlike(s string) []m.EmailData {
	var out []m.EmailData
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ";")
		if len(parts) != 3 {
			// пропускаем строки с неправильным форматом (напр. "RediffMail551")
			continue
		}
		cc := parts[0]
		prov := parts[1]
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		out = append(out, m.EmailData{
			Country:      cc,
			Provider:     prov,
			DeliveryTime: n,
		})
	}
	return out
}

// Хелпер сравнения двух срезов EmailData по (Provider, DeliveryTime)
func equalProviderSlices(got, want []m.EmailData) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].Provider != want[i].Provider || got[i].DeliveryTime != want[i].DeliveryTime {
			return false
		}
	}
	return true
}

const wantStr = `RU;Gmail;23
RU;Yahoo;169
RU;Hotmail;63
RU;MSN;475
RU;Orange;519
RU;Comcast;408
RU;AOL;254
RU;GMX;246`

func TestBuildSortedEmails_RU_FromWantStr(t *testing.T) {
	// Строим срез по «очищенным» данным
	in := parseEmailCSVlike(wantStr)

	got := BuildSortedEmails(in)
	ru, ok := got["RU"]
	if !ok {
		t.Fatalf("RU key not found in result map")
	}
	if len(ru) != 2 {
		t.Fatalf("RU must contain 2 slices [fast, slow], got %d", len(ru))
	}

	// ожидаемые топ-3 быстрых: Gmail(23), Hotmail(63), Yahoo(169)
	wantFast := []m.EmailData{
		{Provider: "Gmail", DeliveryTime: 23},
		{Provider: "Hotmail", DeliveryTime: 63},
		{Provider: "Yahoo", DeliveryTime: 169},
	}
	if !equalProviderSlices(ru[0], wantFast) {
		t.Errorf("fastest mismatch.\n got: %#v\nwant: %#v", ru[0], wantFast)
	}

	// ожидаемые топ-3 медленных (по возрастанию внутри «хвоста»):
	// Comcast(408), MSN(475), Orange(519)
	wantSlow := []m.EmailData{
		{Provider: "Comcast", DeliveryTime: 408},
		{Provider: "MSN", DeliveryTime: 475},
		{Provider: "Orange", DeliveryTime: 519},
	}
	if !equalProviderSlices(ru[1], wantSlow) {
		t.Errorf("slowest mismatch.\n got: %#v\nwant: %#v", ru[1], wantSlow)
	}
}

func TestBuildSortedEmails_US_AveragingAndOrder(t *testing.T) {
	in := []m.EmailData{
		//
		{Country: "US", Provider: "Gmail", DeliveryTime: 120},
		{Country: "US", Provider: "Gmail", DeliveryTime: 180}, // avg 150
		{Country: "US", Provider: "Yahoo", DeliveryTime: 180},
		{Country: "US", Provider: "Hotmail", DeliveryTime: 60},
		{Country: "US", Provider: "AOL", DeliveryTime: 200},
		{Country: "US", Provider: "Proton", DeliveryTime: 90},

		//
		{Country: "DE", Provider: "GMX", DeliveryTime: 100},
		{Country: "DE", Provider: "GMX", DeliveryTime: 100},   // avg 100
		{Country: "DE", Provider: "Gmail", DeliveryTime: 100}, // avg 100
		{Country: "DE", Provider: "Gmail", DeliveryTime: 100}, // avg 100
		{Country: "DE", Provider: "Gmail", DeliveryTime: 100}, // avg 100
		{Country: "DE", Provider: "WebDe", DeliveryTime: 100}, // avg 100

		//
		{Country: "IN", Provider: "Gmail", DeliveryTime: 300},
		{Country: "IN", Provider: "Yahoo", DeliveryTime: 400},
		{Country: "IN", Provider: "Rediff", DeliveryTime: 250},

		//
		{Country: "FR", Provider: "Orange", DeliveryTime: 220},
		{Country: "FR", Provider: "Free", DeliveryTime: 210},
		{Country: "FR", Provider: "SFR", DeliveryTime: 230},
	}

	got := BuildSortedEmails(in)

	// US: ожидания
	us := got["US"]
	if len(us) != 2 {
		t.Fatalf("US must contain 2 slices")
	}
	wantUSFast := []m.EmailData{
		{Provider: "Hotmail", DeliveryTime: 60},
		{Provider: "Proton", DeliveryTime: 90},
		{Provider: "Gmail", DeliveryTime: 150}, // усреднено
	}
	if !equalProviderSlices(us[0], wantUSFast) {
		t.Errorf("US fastest mismatch.\n got: %#v\nwant: %#v", us[0], wantUSFast)
	}
	wantUSSlow := []m.EmailData{
		{Provider: "Gmail", DeliveryTime: 150},
		{Provider: "Yahoo", DeliveryTime: 180},
		{Provider: "AOL", DeliveryTime: 200},
	}
	if !equalProviderSlices(us[1], wantUSSlow) {
		t.Errorf("US slowest mismatch.\n got: %#v\nwant: %#v", us[1], wantUSSlow)
	}

	// DE: все равны → порядок по имени провайдера (алфавит)
	de := got["DE"]
	wantDE := []m.EmailData{
		{Provider: "GMX", DeliveryTime: 100},
		{Provider: "Gmail", DeliveryTime: 100},
		{Provider: "WebDe", DeliveryTime: 100},
	}
	if !equalProviderSlices(de[0], wantDE) {
		t.Errorf("DE tie-break fastest mismatch.\n got: %#v\nwant: %#v", de[0], wantDE)
	}
	if !equalProviderSlices(de[1], wantDE) {
		t.Errorf("DE tie-break slowest mismatch.\n got: %#v\nwant: %#v", de[1], wantDE)
	}

	// IN: проверим обычный «хвост»
	inres := got["IN"]
	// wantINSlow := []m.EmailData{
	// 	{Provider: "Gmail", DeliveryTime: 300},
	// 	{Provider: "Yahoo", DeliveryTime: 400},
	// 	{Provider: "Rediff", DeliveryTime: 250}, // внимание: порядок «хвоста» — по возрастанию последней тройки,
	// }
	// Внимание: наш алгоритм отдаёт последние 3 элементы сортированного списка по возрастанию.
	// Отсортируем ожидание правильно: 250, 300, 400 → «хвост» будет [300, 400] только если элементов >3.
	// Для IN элементов ровно 3, так что и fastest, и slowest будут одинаковыми, просто вся тройка по возрастанию.
	wantIN := []m.EmailData{
		{Provider: "Rediff", DeliveryTime: 250},
		{Provider: "Gmail", DeliveryTime: 300},
		{Provider: "Yahoo", DeliveryTime: 400},
	}
	if !equalProviderSlices(inres[0], wantIN) {
		t.Errorf("IN fastest mismatch.\n got: %#v\nwant: %#v", inres[0], wantIN)
	}
	if !equalProviderSlices(inres[1], wantIN) {
		t.Errorf("IN slowest mismatch.\n got: %#v\nwant: %#v", inres[1], wantIN)
	}

	// FR: простая проверка быстрых
	fr := got["FR"]
	wantFRFast := []m.EmailData{
		{Provider: "Free", DeliveryTime: 210},
		{Provider: "Orange", DeliveryTime: 220},
		{Provider: "SFR", DeliveryTime: 230},
	}
	if !equalProviderSlices(fr[0], wantFRFast) {
		t.Errorf("FR fastest mismatch.\n got: %#v\nwant: %#v", fr[0], wantFRFast)
	}
}

func TestBuildSortedEmails_LessThanThreeProviders(t *testing.T) {
	in := []m.EmailData{
		{Country: "JP", Provider: "Docomo", DeliveryTime: 140},
		{Country: "JP", Provider: "Au", DeliveryTime: 160},
	}
	got := BuildSortedEmails(in)
	jp := got["JP"]
	if len(jp[0]) != 2 || len(jp[1]) != 2 {
		t.Fatalf("with <3 providers both slices should have len=2, got fast=%d slow=%d", len(jp[0]), len(jp[1]))
	}
	// При двух провайдерах fastest и slowest будут одинаковыми (вся отсортированная пара).
	want := []m.EmailData{
		{Provider: "Docomo", DeliveryTime: 140},
		{Provider: "Au", DeliveryTime: 160},
	}
	if !equalProviderSlices(jp[0], want) || !equalProviderSlices(jp[1], want) {
		t.Errorf("JP slices mismatch.\nfast: %#v\nslow: %#v\nwant: %#v", jp[0], jp[1], want)
	}
}

func TestBuildSortedEmails_NormalizationAndBadData(t *testing.T) {
	in := []m.EmailData{
		// Нормализация страны: разные регистры/пробелы → "RU"
		{Country: " ru ", Provider: "Ok", DeliveryTime: 100},
		{Country: "Ru", Provider: "AlsoOk", DeliveryTime: 120},
		{Country: "RU", Provider: "", DeliveryTime: -50}, // плохие данные: пустой провайдер и отрицательное время
	}
	got := BuildSortedEmails(in)

	ru, ok := got["RU"]
	if !ok {
		t.Fatalf("country normalization failed: RU key not found")
	}
	// Проверяем, что «плохая» запись (Provider=="", DeliveryTime==-50) попала первой среди быстрых.
	if len(ru[0]) == 0 || ru[0][0].Provider != "" || ru[0][0].DeliveryTime != -50 {
		t.Errorf("bad data expected to be treated as 'fastest' due to negative time, got: %#v", ru[0])
	}
}

// тестим GoFetch
// deepEqualEmailMap — сравнение map[string][][]EmailData
func deepEqualEmailMap(a, b map[string][][]m.EmailData) bool {
	return reflect.DeepEqual(a, b)
}

// stub logger

func TestGoFetch_SuccessPublishesRanked(t *testing.T) {
	// arrange: подменяем fetchEmails, чтобы вернуть контролируемые данные
	orig := fetchEmails
	defer func() { fetchEmails = orig }()

	sample := []m.EmailData{
		{Country: "RU", Provider: "Gmail", DeliveryTime: 23},
		{Country: "RU", Provider: "Yahoo", DeliveryTime: 169},
		{Country: "RU", Provider: "Hotmail", DeliveryTime: 63},
		{Country: "RU", Provider: "MSN", DeliveryTime: 475},
		{Country: "RU", Provider: "Orange", DeliveryTime: 519},
		{Country: "RU", Provider: "Comcast", DeliveryTime: 408},
		{Country: "RU", Provider: "AOL", DeliveryTime: 254},
		{Country: "RU", Provider: "GMX", DeliveryTime: 246},

		{Country: "US", Provider: "Gmail", DeliveryTime: 120},
		{Country: "US", Provider: "Gmail", DeliveryTime: 180}, // avg 150
		{Country: "US", Provider: "Hotmail", DeliveryTime: 60},
		{Country: "US", Provider: "AOL", DeliveryTime: 200},
	}

	fetchEmails = func(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) ([]m.EmailData, error) {
		return sample, nil
	}

	logger := testLogger()
	cfg := &config.CfgApp{}
	var rs m.ResultSetT
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(context.Background())
	// небольшой таймаут для внутреннего ctx (но он не должен сработать)
	GoFetch(g, ctx, logger, 250*time.Millisecond, cfg, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("GoFetch returned group error: %v", err)
	}

	// expect: rs.Email заполнен BuildSortedEmails(sample)
	want := BuildSortedEmails(sample)

	mu.Lock()
	got := rs.Email
	mu.Unlock()

	if !deepEqualEmailMap(got, want) {
		t.Errorf("rs.Email mismatch.\n got: %#v\nwant: %#v", got, want)
	}
}

func TestGoFetch_FetchError_NoPublish(t *testing.T) {
	orig := fetchEmails
	defer func() { fetchEmails = orig }()

	fetchEmails = func(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) ([]m.EmailData, error) {
		return nil, io.EOF // любая ошибка
	}

	logger := testLogger()
	cfg := &config.CfgApp{}
	var rs m.ResultSetT
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(context.Background())
	GoFetch(g, ctx, logger, 200*time.Millisecond, cfg, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("GoFetch returned group error: %v", err)
	}

	// expect: при ошибке публикации нет (rs.Email остаётся nil)
	mu.Lock()
	defer mu.Unlock()
	if rs.Email != nil && len(rs.Email) != 0 {
		t.Errorf("rs.Email should be nil/empty on fetch error, got: %#v", rs.Email)
	}
}

func TestGoFetch_CancelBeforePublish(t *testing.T) {
	orig := fetchEmails
	defer func() { fetchEmails = orig }()

	// Эмулируем «долгий» Fetch, чтобы таймаут внутри GoFetch успел истечь
	fetchEmails = func(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) ([]m.EmailData, error) {
		time.Sleep(40 * time.Millisecond)
		return []m.EmailData{
			{Country: "RU", Provider: "Gmail", DeliveryTime: 10},
			{Country: "RU", Provider: "Yahoo", DeliveryTime: 20},
			{Country: "RU", Provider: "AOL", DeliveryTime: 30},
			{Country: "RU", Provider: "GMX", DeliveryTime: 40},
		}, nil
	}

	logger := testLogger()
	cfg := &config.CfgApp{}
	var rs m.ResultSetT
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(context.Background())
	// ставим очень маленький timeout, чтобы ctx в GoFetch успел отмениться
	GoFetch(g, ctx, logger, 10*time.Millisecond, cfg, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("GoFetch returned group error: %v", err)
	}

	// expect: отмена до публикации → rs.Email не установлен
	mu.Lock()
	defer mu.Unlock()
	if rs.Email != nil && len(rs.Email) != 0 {
		t.Errorf("expected no publish on cancel-before-publish, got: %#v", rs.Email)
	}
}
