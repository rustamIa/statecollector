package jsonx

import (
	"bytes"
	"errors"
	"io"
	valid "main/internal/validatestruct"
	"strings"
	"testing"
)

// Тестовый тип с Validate() – попадает под интерфейс Validatable.
type SupportData struct {
	Topic         string `json:"topic"       validate:"required"`
	ActiveTickets int    `json:"active_tickets" validate:"gte=0"`
}

func (v SupportData) Validate() error {
	return valid.Struct(v)
}

// Тип без метода Validate() – проверим ветку ValidateFunc из Options[T].
type Row struct {
	A int `json:"a"`
}

func TestDecodeArray_Valid_PreserveOrder(t *testing.T) {
	in := []byte(`[
		{"topic":"SMS","active_tickets":3},
		{"topic":"MMS","active_tickets":9},
		{"topic":"Billing","active_tickets":0}
	]`)

	got, err := DecodeArray[SupportData](in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	if got[0].Topic != "SMS" || got[1].Topic != "MMS" || got[2].Topic != "Billing" {
		t.Fatalf("order mismatch: %+v", got)
	}
}

func TestDecodeArray_UnknownField_SkipByDefault(t *testing.T) {
	in := []byte(`[
		{"topic":"SMS","active_tickets":3},
		{"topic":"MMS","active_tickets":9,"extra":"oops"},
		{"topic":"Billing","active_tickets":0}
	]`)
	got, err := DecodeArray[SupportData](in, nil) // FailFast=false по умолчанию
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// второй элемент должен быть пропущен
	if len(got) != 2 {
		t.Fatalf("expected 2 valid items, got %d", len(got))
	}
	if got[0].Topic != "SMS" || got[1].Topic != "Billing" {
		t.Fatalf("unexpected items: %+v", got)
	}
}

func TestDecodeArray_UnknownField_FailFast(t *testing.T) {
	in := []byte(`[
		{"topic":"SMS","active_tickets":3},
		{"topic":"MMS","active_tickets":9,"extra":"oops"},
		{"topic":"Billing","active_tickets":59}
	]`)
	_, err := DecodeArray(in, &Options[SupportData]{FailFast: true}) //_, err := DecodeArray[SupportData](in, &Options[SupportData]{FailFast: true})
	if err == nil {
		t.Fatalf("expected error on unknown field with FailFast, got nil")
	}
}

func TestDecodeArray_MixedValidInvalid_KeepOnlyValid(t *testing.T) {
	in := []byte(`[
		{"topic":"SMS","active_tickets":-1},
		{"topic":"MMS","active_tickets":9},
		{"topic":"","active_tickets":5},
		{"topic":"Billing","active_tickets":0}
	]`)
	got, err := DecodeArray[SupportData](in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 valid items, got %d", len(got))
	}
	if got[0].Topic != "MMS" || got[1].Topic != "Billing" {
		t.Fatalf("unexpected items: %+v", got)
	}
}

func TestDecodeArray_EmptyArray(t *testing.T) {
	in := []byte(`[]`)
	got, err := DecodeArray[SupportData](in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %d", len(got))
	}
}

func TestDecodeArray_TopLevelNotArray_Error(t *testing.T) {
	in := []byte(`{"topic":"SMS","active_tickets":3}`)
	_, err := DecodeArray[SupportData](in, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrTopLevelNotArray) {
		t.Fatalf("expected ErrTopLevelNotArray, got %v", err)
	}
}

func TestDecodeArray_UnterminatedArray_Error(t *testing.T) {
	in := []byte(`[{"topic":"SMS","active_tickets":3}`)
	_, err := DecodeArray[SupportData](in, nil)
	if err == nil {
		t.Fatalf("expected syntax error for unterminated array, got nil")
	}
	// точный тип ошибки может быть *json.SyntaxError – конкретику не навязываем
}

func TestDecodeArray_ElementTypeMismatch_SkipOrFailFast(t *testing.T) {
	// type mismatch: active_tickets как строка
	in := []byte(`[
		{"topic":"SMS","active_tickets":"bad"},
		{"topic":"MMS","active_tickets":2}
	]`)
	// По умолчанию: первый скип, второй остаётся
	got, err := DecodeArray[SupportData](in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Topic != "MMS" {
		t.Fatalf("expected only MMS to pass, got: %+v", got)
	}
	// FailFast: ошибка на первом элементе
	_, err = DecodeArray[SupportData](in, &Options[SupportData]{FailFast: true})
	if err == nil {
		t.Fatalf("expected error with FailFast on type mismatch, got nil")
	}
}

func TestDecodeArrayFromReader_Basic(t *testing.T) {
	in := []byte(`[
		{"topic":"SMS","active_tickets":3},
		{"topic":"MMS","active_tickets":9}
	]`)
	r := io.NopCloser(bytes.NewReader(in)) // NopCloser – тоже подходит (io.Reader интерфейс)
	got, err := DecodeArrayFromReader[SupportData](r, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
}

func TestDecodeArray_Huge(t *testing.T) {
	var b strings.Builder
	b.WriteString("[")
	const N = 5000
	for i := range N {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"topic":"SMS","active_tickets":1}`)
	}
	b.WriteString("]")

	got, err := DecodeArray[SupportData]([]byte(b.String()), nil)
	if err != nil {
		t.Fatalf("unexpected error on huge array: %v", err)
	}
	if len(got) != N {
		t.Fatalf("expected %d items, got %d", N, len(got))
	}
}

func TestDecodeArray_ValidateFunc_ForTypeWithoutMethod(t *testing.T) {
	in := []byte(`[
		{"a": 1},
		{"a": 2},
		{"a": -4}
	]`)

	// Используем ValidateFunc, потому что Row не реализует Validatable
	got, err := DecodeArray[Row](in, &Options[Row]{
		ValidateFunc: func(r Row) error {
			if r.A%2 != 0 {
				return errors.New("A must be even")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].A != 2 || got[1].A != -4 {
		t.Fatalf("expected [2, -4], got %+v", got)
	}

	// Тот же ввод, но с FailFast: ошибка на первом (1)
	_, err = DecodeArray[Row](in, &Options[Row]{
		ValidateFunc: func(r Row) error {
			if r.A%2 != 0 {
				return errors.New("A must be even")
			}
			return nil
		},
		FailFast: true,
	})
	if err == nil {
		t.Fatalf("expected error with FailFast and invalid first element, got nil")
	}
}
