package smsdata

import (
	"context"
	"errors"
	"log/slog"
	"main/config"
	m "main/internal/model"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// для примера сделан отдельный goFetch - вызов такого будет занимать в RUN меньше места, хотя да он схож шаблону goFetchSlice
// контекст в GoFetch нужен не для mu.Unlock(), а для ранней отмены/таймаута самой работы, чтобы g.Wait() не завис навсегда, если GoFetchSMS подвис
func GoFetch(
	g *errgroup.Group,
	parentCtx context.Context,
	logger *slog.Logger,
	timeout time.Duration,
	cfg *config.CfgApp,
	rs *m.ResultSetT,
	mu *sync.Mutex,
) {
	g.Go(func() error {
		// таймаут на задачу
		ctx := parentCtx
		var cancel context.CancelFunc
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(parentCtx, timeout)
			defer cancel()
		}

		start := time.Now()

		nonSortedData, err := Fetch(ctx, logger, cfg) // []sms.SMSData
		if err != nil {
			// отличаем отмену от реальной ошибки
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Info("sms cancelled", slog.Duration("dur", time.Since(start)))
				return nil
			}
			logger.Info("sms NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil // не валим группу
		}

		// перед публикацией ещё раз убеждаемся, что не отменено
		select {
		case <-ctx.Done():
			logger.Info("sms cancelled before publish", slog.Duration("dur", time.Since(start)))
			return nil
		default:
		}
		//все что ниже продолжит выполнение как по default
		sortedData := BuildSortedSMS(nonSortedData) // [][]sms.SMSData

		// сохранить результат с защитой от гонок
		mu.Lock()
		rs.SMS = sortedData
		mu.Unlock()

		// посчитать реальное количество строк во всех под-срезах
		total := 0
		for _, part := range sortedData {
			total += len(part)
		}

		logger.Info("sms fetched",
			slog.Int("count", total),
			slog.Duration("dur", time.Since(start)),
		)
		logger.Debug("sms data:", " ", sortedData)
		return nil
	})
}

func countryName(alpha2 string) string {
	if n, ok := countryNames[strings.ToUpper(strings.TrimSpace(alpha2))]; ok {
		return n
	}
	return alpha2
}

// BuildSortedSMS:
// 1) подменяет Country: alpha-2 → полное название,
// 2) готовит два набора:
//   - по провайдеру A→Z,
//   - по стране A→Z,
//
// 3) объединяет в [][]SMSData, где [0] — сортировка по провайдеру, [1] — по стране.
//
// ВАЖНО: валидацию вы уже прошли в Fetch (там Country — alpha-2).
// После подмены на полные названия повторно Validate() вызывать не нужно.
func BuildSortedSMS(in []m.SMSData) [][]m.SMSData {
	// 1) нормализуем страны (делаем копию входного среза)
	mapped := make([]m.SMSData, len(in))
	copy(mapped, in)
	for i := range mapped {
		mapped[i].Country = countryName(mapped[i].Country)
	}

	// 2) сортировка по провайдеру (A→Z)
	byProvider := make([]m.SMSData, len(mapped))
	copy(byProvider, mapped)

	slices.SortStableFunc(byProvider, func(a, b m.SMSData) int {
		return strings.Compare(a.Provider, b.Provider) //не учитывал strings.ToLower, может и стоит
	})

	// 3) сортировка по стране (A→Z)
	byCountry := make([]m.SMSData, len(mapped))
	copy(byCountry, mapped)

	slices.SortStableFunc(byCountry, func(a, b m.SMSData) int {
		return strings.Compare(a.Country, b.Country)
	})

	// 4) объединяем
	return [][]m.SMSData{byProvider, byCountry}
}

// --- справочник стран (alpha-2 → полное название) и хелпер ---
var countryNames = map[string]string{
	"AD": "Andorra",
	"AE": "United Arab Emirates",
	"AF": "Afghanistan",
	"AG": "Antigua and Barbuda",
	"AI": "Anguilla",
	"AL": "Albania",
	"AM": "Armenia",
	"AO": "Angola",
	"AQ": "Antarctica",
	"AR": "Argentina",
	"AS": "American Samoa",
	"AT": "Austria",
	"AU": "Australia",
	"AW": "Aruba",
	"AX": "Aland Islands",
	"AZ": "Azerbaijan",

	"BA": "Bosnia and Herzegovina",
	"BB": "Barbados",
	"BD": "Bangladesh",
	"BE": "Belgium",
	"BF": "Burkina Faso",
	"BG": "Bulgaria",
	"BH": "Bahrain",
	"BI": "Burundi",
	"BJ": "Benin",
	"BL": "Saint Barthelemy",
	"BM": "Bermuda",
	"BN": "Brunei",
	"BO": "Bolivia",
	"BQ": "Bonaire, Sint Eustatius and Saba",
	"BR": "Brazil",
	"BS": "Bahamas",
	"BT": "Bhutan",
	"BV": "Bouvet Island",
	"BW": "Botswana",
	"BY": "Belarus",
	"BZ": "Belize",

	"CA": "Canada",
	"CC": "Cocos (Keeling) Islands",
	"CD": "Congo (DRC)",
	"CF": "Central African Republic",
	"CG": "Congo",
	"CH": "Switzerland",
	"CI": "Cote d'Ivoire",
	"CK": "Cook Islands",
	"CL": "Chile",
	"CM": "Cameroon",
	"CN": "China",
	"CO": "Colombia",
	"CR": "Costa Rica",
	"CU": "Cuba",
	"CV": "Cabo Verde",
	"CW": "Curacao",
	"CX": "Christmas Island",
	"CY": "Cyprus",
	"CZ": "Czechia",

	"DE": "Germany",
	"DJ": "Djibouti",
	"DK": "Denmark",
	"DM": "Dominica",
	"DO": "Dominican Republic",
	"DZ": "Algeria",

	"EC": "Ecuador",
	"EE": "Estonia",
	"EG": "Egypt",
	"EH": "Western Sahara",
	"ER": "Eritrea",
	"ES": "Spain",
	"ET": "Ethiopia",

	"FI": "Finland",
	"FJ": "Fiji",
	"FK": "Falkland Islands",
	"FM": "Micronesia",
	"FO": "Faroe Islands",
	"FR": "France",

	"GA": "Gabon",
	"GB": "United Kingdom",
	"GD": "Grenada",
	"GE": "Georgia",
	"GF": "French Guiana",
	"GG": "Guernsey",
	"GH": "Ghana",
	"GI": "Gibraltar",
	"GL": "Greenland",
	"GM": "Gambia",
	"GN": "Guinea",
	"GP": "Guadeloupe",
	"GQ": "Equatorial Guinea",
	"GR": "Greece",
	"GS": "South Georgia and South Sandwich Islands",
	"GT": "Guatemala",
	"GU": "Guam",
	"GW": "Guinea-Bissau",
	"GY": "Guyana",

	"HK": "Hong Kong",
	"HM": "Heard Island and McDonald Islands",
	"HN": "Honduras",
	"HR": "Croatia",
	"HT": "Haiti",
	"HU": "Hungary",

	"ID": "Indonesia",
	"IE": "Ireland",
	"IL": "Israel",
	"IM": "Isle of Man",
	"IN": "India",
	"IO": "British Indian Ocean Territory",
	"IQ": "Iraq",
	"IR": "Iran",
	"IS": "Iceland",
	"IT": "Italy",

	"JE": "Jersey",
	"JM": "Jamaica",
	"JO": "Jordan",
	"JP": "Japan",

	"KE": "Kenya",
	"KG": "Kyrgyzstan",
	"KH": "Cambodia",
	"KI": "Kiribati",
	"KM": "Comoros",
	"KN": "Saint Kitts and Nevis",
	"KP": "North Korea",
	"KR": "South Korea",
	"KW": "Kuwait",
	"KY": "Cayman Islands",
	"KZ": "Kazakhstan",

	"LA": "Laos",
	"LB": "Lebanon",
	"LC": "Saint Lucia",
	"LI": "Liechtenstein",
	"LK": "Sri Lanka",
	"LR": "Liberia",
	"LS": "Lesotho",
	"LT": "Lithuania",
	"LU": "Luxembourg",
	"LV": "Latvia",
	"LY": "Libya",

	"MA": "Morocco",
	"MC": "Monaco",
	"MD": "Moldova",
	"ME": "Montenegro",
	"MF": "Saint Martin",
	"MG": "Madagascar",
	"MH": "Marshall Islands",
	"MK": "North Macedonia",
	"ML": "Mali",
	"MM": "Myanmar",
	"MN": "Mongolia",
	"MO": "Macao",
	"MP": "Northern Mariana Islands",
	"MQ": "Martinique",
	"MR": "Mauritania",
	"MS": "Montserrat",
	"MT": "Malta",
	"MU": "Mauritius",
	"MV": "Maldives",
	"MW": "Malawi",
	"MX": "Mexico",
	"MY": "Malaysia",
	"MZ": "Mozambique",

	"NA": "Namibia",
	"NC": "New Caledonia",
	"NE": "Niger",
	"NF": "Norfolk Island",
	"NG": "Nigeria",
	"NI": "Nicaragua",
	"NL": "Netherlands",
	"NO": "Norway",
	"NP": "Nepal",
	"NR": "Nauru",
	"NU": "Niue",
	"NZ": "New Zealand",

	"OM": "Oman",

	"PA": "Panama",
	"PE": "Peru",
	"PF": "French Polynesia",
	"PG": "Papua New Guinea",
	"PH": "Philippines",
	"PK": "Pakistan",
	"PL": "Poland",
	"PM": "Saint Pierre and Miquelon",
	"PN": "Pitcairn",
	"PR": "Puerto Rico",
	"PS": "Palestine",
	"PT": "Portugal",
	"PW": "Palau",
	"PY": "Paraguay",

	"QA": "Qatar",

	"RE": "Reunion",
	"RO": "Romania",
	"RS": "Serbia",
	"RU": "Russia",
	"RW": "Rwanda",

	"SA": "Saudi Arabia",
	"SB": "Solomon Islands",
	"SC": "Seychelles",
	"SD": "Sudan",
	"SE": "Sweden",
	"SG": "Singapore",
	"SH": "Saint Helena, Ascension and Tristan da Cunha",
	"SI": "Slovenia",
	"SJ": "Svalbard and Jan Mayen",
	"SK": "Slovakia",
	"SL": "Sierra Leone",
	"SM": "San Marino",
	"SN": "Senegal",
	"SO": "Somalia",
	"SR": "Suriname",
	"SS": "South Sudan",
	"ST": "Sao Tome and Principe",
	"SV": "El Salvador",
	"SX": "Sint Maarten",
	"SY": "Syria",
	"SZ": "Eswatini",

	"TC": "Turks and Caicos Islands",
	"TD": "Chad",
	"TF": "French Southern Territories",
	"TG": "Togo",
	"TH": "Thailand",
	"TJ": "Tajikistan",
	"TK": "Tokelau",
	"TL": "Timor-Leste",
	"TM": "Turkmenistan",
	"TN": "Tunisia",
	"TO": "Tonga",
	"TR": "Turkiye",
	"TT": "Trinidad and Tobago",
	"TV": "Tuvalu",
	"TW": "Taiwan",
	"TZ": "Tanzania",

	"UA": "Ukraine",
	"UG": "Uganda",
	"UM": "United States Minor Outlying Islands",
	"US": "United States",
	"UY": "Uruguay",
	"UZ": "Uzbekistan",

	"VA": "Vatican City",
	"VC": "Saint Vincent and the Grenadines",
	"VE": "Venezuela",
	"VG": "British Virgin Islands",
	"VI": "United States Virgin Islands",
	"VN": "Vietnam",
	"VU": "Vanuatu",

	"WF": "Wallis and Futuna",
	"WS": "Samoa",

	"YE": "Yemen",
	"YT": "Mayotte",

	"ZA": "South Africa",
	"ZM": "Zambia",
	"ZW": "Zimbabwe",
}
