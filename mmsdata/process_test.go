package mmsdata

import (
	"cmp"
	"reflect"
	"slices"
	"strings"
	"testing"

	countries "main/internal/alpha2"
	m "main/internal/model"
)

// вспомогалка
func isSortedStrings(xs []string) bool {
	return slices.IsSortedFunc(xs, func(a, b string) int { return cmp.Compare(a, b) })
}

func TestBuildSortedMMS_Empty(t *testing.T) {
	in := []m.MMSData{}
	out := BuildSortedMMS(in)

	if len(out) != 2 {
		t.Fatalf("ожидали 2 среза, получили %d", len(out))
	}
	if got := len(out[0]); got != 0 {
		t.Fatalf("byProvider должен быть пуст, len=%d", got)
	}
	if got := len(out[1]); got != 0 {
		t.Fatalf("byCountry должен быть пуст, len=%d", got)
	}
}

func TestBuildSortedMMS_WithBadishDataset(t *testing.T) {
	// Данные «как есть» из условия. Bandwidth/response_time/провайдеры могут быть «плохими»,
	// но BuildSortedMMS только нормализует страны и сортирует.
	in := []m.MMSData{
		{Country: "US", Provider: "Rond", Bandwidth: "36", ResponseTime: "1576"},
		{Country: "USA", Provider: "Rond", Bandwidth: "50", ResponseTime: "100"},   // bad ISO2
		{Country: "DE", Provider: "Bad", Bandwidth: "10", ResponseTime: "200"},     // bad provider (но нас это не волнует здесь)
		{Country: "FR", Provider: "Topolo", Bandwidth: "200", ResponseTime: "300"}, // >100
		{Country: "GB", Provider: "Kildy", Bandwidth: "ok", ResponseTime: "300"},   // not a number
		{Country: "GB", Provider: "Kildy", Bandwidth: "85", ResponseTime: "300"},   // имитируем запись с "x":1 — в структуру не попадёт
		{Country: "GB", Provider: "Kildy", Bandwidth: "85", ResponseTime: "300"},
	}

	orig := append([]m.MMSData(nil), in...) // in... значит “возьми все элементы среза in и передай/добавь их по одному”
	out := BuildSortedMMS(in)
	if len(out) != 2 {
		t.Fatalf("ожидали 2 среза, получили %d", len(out))
	}
	if !reflect.DeepEqual(in, orig) {
		t.Fatalf("входной срез был изменён функцией")
	}

	byProvider := out[0]
	byCountry := out[1]

	// 1) Проверка сортировки по провайдеру (A→Z) без предположений о регистре
	gotProviders := make([]string, len(byProvider))
	for i, r := range byProvider {
		gotProviders[i] = r.Provider
	}
	sortedProviders := append([]string(nil), gotProviders...)
	slices.SortFunc(sortedProviders, strings.Compare)
	if !reflect.DeepEqual(gotProviders, sortedProviders) {
		t.Fatalf("byProvider не отсортирован лексикографически\n got=%v\nexp=%v", gotProviders, sortedProviders)
	}

	// 1a) Проверим стабильность для одинакового провайдера "Kildy":
	// в исходном in порядок Kildy записей: ["ok", "85", "85"].
	var kildyOrder []string
	for _, r := range byProvider {
		if r.Provider == "Kildy" {
			kildyOrder = append(kildyOrder, r.Bandwidth)
		}
	}
	wantKildyPrefix := []string{"ok", "85"} // как минимум первые две должны сохранить относительный порядок
	if len(kildyOrder) < 2 || !reflect.DeepEqual(kildyOrder[:2], wantKildyPrefix) {
		t.Fatalf("нестабильная сортировка для Provider=Kildy: got=%v, want префикс=%v", kildyOrder, wantKildyPrefix)
	}

	// 2) Проверка нормализации стран: каждая Country должна равняться countries.CountryName(исходная)
	for i, r := range byCountry {
		//exp := countries.CountryName(orig[i].Country) // NB: сравнивать с тем же индексом нельзя (byCountry отсортирован)
		// Поэтому проверим на всём массиве: для каждого элемента byCountry существует хотя бы один исходный,
		// у которого CountryName совпадает.
		found := false
		for _, src := range orig {
			if r.Country == countries.CountryName(src.Country) &&
				r.Provider == src.Provider &&
				r.Bandwidth == src.Bandwidth &&
				r.ResponseTime == src.ResponseTime {
				found = true
				break
			}
		}
		if !found && i >= 0 { // i>=0 лишь чтобы подавить линтер о неиспользуемой переменной
			t.Fatalf("элемент %+v не соответствует нормализованным исходным данным", r)
		}
	}

	// 3) Проверка, что byCountry действительно отсортирован по Country (A→Z)
	gotCountries := make([]string, len(byCountry))
	for i, r := range byCountry {
		gotCountries[i] = r.Country
	}
	if !isSortedStrings(gotCountries) {
		t.Fatalf("byCountry не отсортирован по стране (A→Z): %v", gotCountries)
	}
}

func TestBuildSortedMMS_WithGoodISO2Sample(t *testing.T) {
	// «Хорошие» ISO2 коды: AD, AE, AF, AG, AI, AL, AM, BR, BS
	// Ожидаемые нормализованные имена стран:
	wantNames := map[string]string{
		"AD": "Andorra",
		"AE": "United Arab Emirates",
		"AF": "Afghanistan",
		"AG": "Antigua and Barbuda",
		"AI": "Anguilla",
		"AL": "Albania",
		"AM": "Armenia",
		"BR": "Brazil",
		"BS": "Bahamas",
	}

	// Сконструируем данные с повторяющимися провайдерами для проверки стабильности.
	in := []m.MMSData{
		{Country: "AD", Provider: "Zed", Bandwidth: "10", ResponseTime: "100"},
		{Country: "AE", Provider: "Ada", Bandwidth: "20", ResponseTime: "200"},
		{Country: "AF", Provider: "Mid", Bandwidth: "30", ResponseTime: "300"},
		{Country: "AG", Provider: "Ada", Bandwidth: "40", ResponseTime: "400"},
		{Country: "AI", Provider: "Mid", Bandwidth: "50", ResponseTime: "500"},
		{Country: "AL", Provider: "Zed", Bandwidth: "60", ResponseTime: "600"},
		{Country: "AM", Provider: "Ada", Bandwidth: "70", ResponseTime: "700"},
		{Country: "BR", Provider: "Mid", Bandwidth: "80", ResponseTime: "800"},
		{Country: "BS", Provider: "Zed", Bandwidth: "90", ResponseTime: "900"},
	}

	out := BuildSortedMMS(in)
	if len(out) != 2 {
		t.Fatalf("ожидали 2 среза, получили %d", len(out))
	}
	byProvider := out[0]
	byCountry := out[1]

	// A) Проверим, что нормализация стран даёт ожидаемые имена.
	for _, r := range byCountry {
		codeUpper := strings.ToUpper(r.Country) // r.Country уже нормализован — это имя страны
		// Пройдёмся по входу, найдём соответствующий код и проверим ожидание.
		found := false
		for _, src := range in {
			if r.Provider == src.Provider && r.Bandwidth == src.Bandwidth && r.ResponseTime == src.ResponseTime {
				want := wantNames[src.Country]
				got := r.Country
				if want == "" {
					t.Fatalf("в тест-карте нет ожидания для кода %q", src.Country)
				}
				if got != want {
					t.Fatalf("нормализация страны: got=%q, want=%q (код=%s)", got, want, src.Country)
				}
				found = true
				break
			}
		}
		if !found && codeUpper != "" {
			t.Fatalf("не нашли соответствующий исходный элемент для %+v", r)
		}
	}

	// B) Проверим, что byCountry отсортирован по названиям стран (A→Z).
	var gotCountries []string
	for _, r := range byCountry {
		gotCountries = append(gotCountries, r.Country)
	}
	if !isSortedStrings(gotCountries) {
		t.Fatalf("byCountry не отсортирован A→Z: %v", gotCountries)
	}

	// C) Проверим сортировку и стабильность по провайдерам:
	// провайдеры ожидаемо в порядке Ada < Mid < Zed
	var gotProviders []string
	for _, r := range byProvider {
		gotProviders = append(gotProviders, r.Provider)
	}
	if !isSortedStrings(gotProviders) {
		t.Fatalf("byProvider не отсортирован A→Z: %v", gotProviders)
	}
	// Стабильность для "Ada": по входу порядок стран для Ada: AE -> AG -> AM
	var adaCountries []string
	for _, r := range byProvider {
		if r.Provider == "Ada" {
			adaCountries = append(adaCountries, r.Country)
		}
	}
	if !reflect.DeepEqual(adaCountries, []string{
		wantNames["AE"], wantNames["AG"], wantNames["AM"],
	}) {
		t.Fatalf("нестабильная сортировка для Provider=Ada; got=%v", adaCountries)
	}
}

func TestBuildSortedMMS_DoesNotMutateInput(t *testing.T) {
	in := []m.MMSData{
		{Country: "GB", Provider: "Kildy", Bandwidth: "1", ResponseTime: "1"},
		{Country: "GB", Provider: "Kildy", Bandwidth: "2", ResponseTime: "2"},
	}
	orig := append([]m.MMSData(nil), in...)

	_ = BuildSortedMMS(in)

	if !reflect.DeepEqual(in, orig) {
		t.Fatalf("ожидали, что входной срез не изменится")
	}
}
