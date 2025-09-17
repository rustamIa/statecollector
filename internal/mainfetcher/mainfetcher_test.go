package mainfetcher

import (
	"context"
	m "main/internal/model"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// okFetcher «заглушка-фетчер» для тестов, которая имитирует успешную задачу с небольшой задержкой и умеет корректно реагировать на отмену контекста.
func okFetcher(delay time.Duration, calls *atomic.Int32) fetcher { //atomic.Int32, чтобы не было  гонок
	return func(g *errgroup.Group, ctx context.Context) {
		g.Go(func() error {
			select {
			case <-time.After(delay): // имитируем работу N миллисекунд
				calls.Add(1) // считаем, что задача успешно отработала
				return nil   // успех
			case <-ctx.Done(): // если пришла отмена/таймаут
				return ctx.Err() // выходим с ошибкой контекста
			}
		})
	}
}

func errFetcher(delay time.Duration) fetcher {
	return func(g *errgroup.Group, ctx context.Context) {
		g.Go(func() error {
			time.Sleep(delay)
			return nil //errors.New("boom") -так чтобы errgroup не завершил все осталыние горутины
		})
	}
}

func TestGetResultData_HappyPath(t *testing.T) {
	var calls atomic.Int32
	fetchers := []fetcher{
		okFetcher(30*time.Millisecond, &calls),
		okFetcher(40*time.Millisecond, &calls),
		okFetcher(10*time.Millisecond, &calls),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, _ = GetResultData(ctx, nil, nil, fetchers...)

	if got := calls.Load(); got != 3 {
		t.Fatalf("want 3 fetchers called, got %d", got)
	}
}

func TestGetResultData_PartialError(t *testing.T) {
	var calls atomic.Int32
	fetchers := []fetcher{
		okFetcher(10*time.Millisecond, &calls),
		errFetcher(20 * time.Millisecond),
		okFetcher(30*time.Millisecond, &calls),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, _ = GetResultData(ctx, nil, nil, fetchers...) // игнорируем ошибку g.Wait()

	if got := calls.Load(); got != 2 {
		t.Fatalf("want 2 successful fetchers, got %d", got)
	}
}

func cancelAwareFetcher(block time.Duration, entered *atomic.Int32) fetcher {
	return func(g *errgroup.Group, ctx context.Context) {
		g.Go(func() error {
			entered.Add(1)
			select {
			case <-time.After(block):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
}

func TestGetResultData_Cancel(t *testing.T) {
	var entered atomic.Int32
	fetchers := []fetcher{
		cancelAwareFetcher(500*time.Millisecond, &entered),
		cancelAwareFetcher(500*time.Millisecond, &entered),
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, _ = GetResultData(ctx, nil, nil, fetchers...)
	if time.Since(start) > 300*time.Millisecond {
		t.Fatalf("GetResultData should return shortly after cancel")
	}
	// оба фетчера стартовали и увидели cancel
	if got := entered.Load(); got != 2 {
		t.Fatalf("want 2 fetchers entered, got %d", got)
	}
}

// --- тесты ф-ий проверки данных в результирующей структуре ResultSetT и ResultSet
// nonZeroBilling должен вернуть НЕнулевое значение BillingData для "успешных" тестов.
func nonZeroBilling() m.BillingData {
	return m.BillingData{CreateCustomer: true,
		Purchase:     true,
		Payout:       true,
		Recurring:    true,
		FraudControl: true,
		CheckoutPage: true}
}

// validResultSet формирует валидный набор данных.
func validResultSet(t *testing.T) m.ResultSetT {
	t.Helper()

	rs := m.ResultSetT{ //данные квази-пустые, len покажет = 1
		SMS:       [][]m.SMSData{[]m.SMSData{m.SMSData{}}},
		MMS:       [][]m.MMSData{[]m.MMSData{m.MMSData{}}},
		VoiceCall: []m.VoiceCallData{m.VoiceCallData{}},
		Email:     map[string][][]m.EmailData{"ru": [][]m.EmailData{[]m.EmailData{m.EmailData{}}}},
		Billing:   nonZeroBilling(),
		Support:   []int{1},
		Incidents: []m.IncidentData{m.IncidentData{}},
	}

	return rs
}

func TestBuildResultT_Ok(t *testing.T) {
	rs := validResultSet(t)
	got := BuildResultT(rs)

	if !got.Status {
		t.Fatalf("Status=false, want true")
	}
	if got.Error != "" {
		t.Fatalf("Error=%q, want empty", got.Error)
	}
	if !reflect.DeepEqual(got.Data, rs) {
		t.Fatalf("Data mismatch:\n got: %#v\nwant: %#v", got.Data, rs)
	}
}

func TestBuildResultT_Error(t *testing.T) {
	// Возьмём валидный и «сломаем» одно поле.
	rs := m.ResultSetT{
		SMS:       [][]m.SMSData{[]m.SMSData{m.SMSData{}}},
		MMS:       [][]m.MMSData{[]m.MMSData{m.MMSData{}}},
		VoiceCall: []m.VoiceCallData{m.VoiceCallData{}},
		Email:     map[string][][]m.EmailData{"ru": [][]m.EmailData{[]m.EmailData{m.EmailData{}}}},
		Billing:   m.BillingData{}, // сделаем нулевым, чтобы триггернуть ошибку
		Support:   []int{1},
		Incidents: []m.IncidentData{m.IncidentData{}},
	}

	got := BuildResultT(rs)
	if got.Status {
		t.Fatalf("Status=true, want false")
	}
	if got.Error != "Error on collect data" {
		t.Fatalf("Error=%q, want %q", got.Error, "Error on collect data")
	}
	// Data должен остаться нулевым значением структуры.
	if !reflect.DeepEqual(got.Data, m.ResultSetT{}) {
		t.Fatalf("Data must be zero value when error; got=%#v", got.Data)
	}
}

func TestValidateResultSet_Table(t *testing.T) {
	type tc struct {
		name    string
		mutate  func(*m.ResultSetT)
		wantErr string // подстрока ожидаемой ошибки (пустая = ошибок не должно быть)
	}

	tests := []tc{
		{
			name:    "ok_all_filled",
			mutate:  nil,
			wantErr: "",
		},
		// SMS
		{
			name: "sms_empty",
			mutate: func(rs *m.ResultSetT) {
				rs.SMS = nil
			},
			wantErr: "sms empty",
		},
		{
			name: "sms_has_empty_batch",
			mutate: func(rs *m.ResultSetT) {
				rs.SMS = [][]m.SMSData{[]m.SMSData{}}
			},
			wantErr: "sms has empty batch",
		},
		// MMS
		{
			name: "mms_empty",
			mutate: func(rs *m.ResultSetT) {
				rs.MMS = nil
			},
			wantErr: "mms empty",
		},
		{
			name: "mms_has_empty_batch",
			mutate: func(rs *m.ResultSetT) {
				rs.MMS = [][]m.MMSData{[]m.MMSData{}}
			},
			wantErr: "mms has empty batch",
		},
		// VoiceCall
		{
			name: "voice_call_empty",
			mutate: func(rs *m.ResultSetT) {
				rs.VoiceCall = nil
			},
			wantErr: "voice_call empty",
		},
		// Email
		{
			name: "email_nil",
			mutate: func(rs *m.ResultSetT) {
				rs.Email = nil
			},
			wantErr: "email empty",
		},
		{
			name: "email_empty_map",
			mutate: func(rs *m.ResultSetT) {
				rs.Email = map[string][][]m.EmailData{}
			},
			wantErr: "email empty",
		},
		{
			name: "email_empty_buckets",
			mutate: func(rs *m.ResultSetT) {
				rs.Email = map[string][][]m.EmailData{"ru": nil}
			},
			wantErr: "email has empty buckets",
		},
		{
			name: "email_has_empty_bucket",
			mutate: func(rs *m.ResultSetT) {
				rs.Email = map[string][][]m.EmailData{"ru": {[]m.EmailData{}}}
			},
			wantErr: "email has empty bucket",
		},
		// Billing
		{
			name: "billing_zero",
			mutate: func(rs *m.ResultSetT) {
				rs.Billing = m.BillingData{} // нулевое значение
			},
			wantErr: "billing is zero",
		},
		// Support
		{
			name: "support_empty",
			mutate: func(rs *m.ResultSetT) {
				rs.Support = nil
			},
			wantErr: "support empty",
		},
		// Incidents
		{
			name: "incident_empty",
			mutate: func(rs *m.ResultSetT) {
				rs.Incidents = nil
			},
			wantErr: "incident empty",
		},
	}

	base := validResultSet(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := base
			if tt.mutate != nil {
				tt.mutate(&rs)
			}
			err := validateResultSet(rs)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain expected %q", err.Error(), tt.wantErr)
			}
		})
	}
}
