package smsdata

import (
	countries "main/internal/alpha2"
	m "main/internal/model"
	"slices"
	"testing"
)

func providersOf(xs []m.SMSData) []string {
	out := make([]string, len(xs))
	for i := range xs {
		out[i] = xs[i].Provider
	}
	return out
}

func countriesOf(xs []m.SMSData) []string {
	out := make([]string, len(xs))
	for i := range xs {
		out[i] = xs[i].Country
	}
	return out
}

func TestCountryName(t *testing.T) {
	tcs := []struct {
		in   string
		want string
	}{
		{"us", "United States"},        // регистронезависимо
		{" DE ", "Germany"},            // тримминг
		{"zz", "zz"},                   // неизвестный код -> как есть
		{"", ""},                       // пустая строка
		{"Ae", "United Arab Emirates"}, // смешанный регистр
	}

	for _, tc := range tcs {
		got := countries.CountryName(tc.in)
		if got != tc.want {
			t.Fatalf("countryName(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildSortedSMS_SortsAndMaps(t *testing.T) {
	in := []m.SMSData{
		{Provider: "Twilio", Country: "us"},
		{Provider: "telnyx", Country: "DE"},
		{Provider: "AWS", Country: "ae"},
		{Provider: "AWS", Country: "US"},  // проверим стабильность внутри одинаковых провайдеров
		{Provider: "aero", Country: "ZZ"}, // неизвестная страна остаётся "ZZ"
	}

	// сохраним копию для проверки неизменности входа
	orig := make([]m.SMSData, len(in))
	copy(orig, in)

	got := BuildSortedSMS(in)
	if len(got) != 2 {
		t.Fatalf("BuildSortedSMS returned %d slices; want 2", len(got))
	}
	byProvider := got[0]
	byCountry := got[1]

	// 1) входной срез не должен быть изменён
	if !slices.EqualFunc(in, orig, func(a, b m.SMSData) bool { return a.Provider == b.Provider && a.Country == b.Country }) {
		t.Fatalf("input slice was modified: got %+v; want %+v", in, orig)
	}

	// 2) проверим, что страны заменены на полные названия в обоих результатах
	for i, s := range [][]m.SMSData{byProvider, byCountry} {
		_ = i
		for _, row := range s {
			switch row.Country {
			case "United States", "Germany", "United Arab Emirates", "ZZ":
				// ок
			default:
				t.Fatalf("expected mapped country name, got %q", row.Country)
			}
		}
	}

	// 3) сортировка по провайдеру — текущая логика чувствительна к регистру (strings.Compare)
	// ASCII-порядок: "AWS" < "AWS" < "Twilio" < "aero" < "telnyx"
	wantProviders := []string{"AWS", "AWS", "Twilio", "aero", "telnyx"}
	if gotProviders := providersOf(byProvider); !slices.Equal(gotProviders, wantProviders) {
		t.Fatalf("byProvider providers = %v; want %v", gotProviders, wantProviders)
	}

	// стабильность: два "AWS" должны сохранить исходный порядок (сначала AE, потом US)
	if !(byProvider[0].Country == "United Arab Emirates" && byProvider[1].Country == "United States") {
		t.Fatalf("stable sort violated for equal providers AWS: got %q then %q",
			byProvider[0].Country, byProvider[1].Country)
	}

	// 4) сортировка по стране после маппинга (также чувствительна к регистру)
	// Полные названия: Germany, United Arab Emirates, United States, United States, ZZ
	wantCountries := []string{"Germany", "United Arab Emirates", "United States", "United States", "ZZ"}
	if gotCountries := countriesOf(byCountry); !slices.Equal(gotCountries, wantCountries) {
		t.Fatalf("byCountry countries = %v; want %v", gotCountries, wantCountries)
	}
}

func TestBuildSortedSMS_Empty(t *testing.T) {
	got := BuildSortedSMS(nil)
	if len(got) != 2 {
		t.Fatalf("BuildSortedSMS(nil) returned %d slices; want 2", len(got))
	}
	if len(got[0]) != 0 || len(got[1]) != 0 {
		t.Fatalf("expected two empty slices, got %v and %v", got[0], got[1])
	}
}
