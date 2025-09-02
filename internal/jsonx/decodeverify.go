package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var ErrTopLevelNotArray = errors.New("jsonx: expected top-level JSON array")

// Если тип умеет сам себя валидировать – пусть реализует Validate()
type Validatable interface {
	Validate() error
}

// Параметры поведения декодера
type Options[T any] struct {
	// Необязательная функция валидации, если тип не реализует Validatable
	ValidateFunc func(T) error
	// Если true — при ошибке элемента сразу возвращаем ошибку;
	// по умолчанию false: пропускаем плохие элементы.
	FailFast bool
}

// DecodeArray: из []byte в []T, строгий разбор каждого элемента с DisallowUnknownFields.
// Если у тебя уже есть []byte (например, в тесте).
// Нужно один раз прочитать тело полностью (логирование, подпись, ретраи с повторным парсом).
// Если Маленькие ответы — чтобы не заморачиваться.
func DecodeArray[T any](data []byte, opt *Options[T]) ([]T, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	return decodeArrayWithDecoder[T](dec, opt)
}

// DecodeArrayFromReader (io.Reader):
// Если парсишь напрямую из сети: jsonx.DecodeArrayFromReader(res.Body, ...).
// Важна пиковая память: не держим весь JSON, буферим лишь по одному элементу (наш декодер читает элемент → валидирует → добавляет → следующий).
// Большие ответы и backpressure: читаем по мере разбора, не вызываем io.ReadAll.
func DecodeArrayFromReader[T any](r io.Reader, opt *Options[T]) ([]T, error) {
	dec := json.NewDecoder(r)
	return decodeArrayWithDecoder[T](dec, opt)
}

func decodeArrayWithDecoder[T any](dec *json.Decoder, opt *Options[T]) ([]T, error) {
	// ждём '['
	tok, err := dec.Token()
	if err != nil {
		return nil, err // здесь Decoder вернёт *json.SyntaxError, если JSON битый
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '[' {
		// своей ошибкой понятнее подсветить контракт
		return nil, fmt.Errorf("%w (got %v at offset %d)", ErrTopLevelNotArray, tok, dec.InputOffset())
	}

	out := make([]T, 0, 8)

	for dec.More() {
		// берём следующий элемент как raw, затем разбираем его строго
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if opt != nil && opt.FailFast {
				return nil, err
			}
			continue
		}

		var v T
		elDec := json.NewDecoder(bytes.NewReader(raw))
		elDec.DisallowUnknownFields()
		if err := elDec.Decode(&v); err != nil {
			if opt != nil && opt.FailFast {
				return nil, err
			}
			continue
		}

		if vv, ok := any(v).(Validatable); ok {
			if err := vv.Validate(); err != nil {
				if opt != nil && opt.FailFast {
					return nil, err
				}
				continue
			}
		} else if opt != nil && opt.ValidateFunc != nil {
			if err := opt.ValidateFunc(v); err != nil {
				if opt.FailFast {
					return nil, err
				}
				continue
			}
		}

		out = append(out, v)
	}

	// ждём ']'
	tok, err = dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != ']' {
		return nil, fmt.Errorf("jsonx: unterminated array (offset %d)", dec.InputOffset())
	}

	return out, nil
}

// var decodeData = decodeJsonAndValidate

// // Вызов метода валидации структуры
// func (v SupportData) Validate() error {
// 	return validate.Struct(v)
// }

// func decodeJsonAndValidate(jsonData []byte) ([]SupportData, error) {
// 	var raws []json.RawMessage
// 	if err := json.Unmarshal(jsonData, &raws); err != nil {
// 		return []SupportData{}, err
// 	}
// 	out := make([]SupportData, 0, len(raws))
// 	for _, rm := range raws {
// 		var item SupportData
// 		// json.NewDecoder даёт два плюса, которых нет у Unmarshal:  DisallowUnknownFields()- парсинг упадёт, если в JSON есть поле, которого нет в MMSData; Через Decoder можно, например, читать не только целиком объект, но и токен за токеном
// 		dec := json.NewDecoder(bytes.NewReader(rm)) //bytes.NewReader(rm) оборачивает слайс в объект, который реализует io.Reader, чтобы можно было работать как с потоком.
// 		dec.DisallowUnknownFields()
// 		if err := dec.Decode(&item); err != nil { //err если есть лишние поля
// 			continue
// 		}
// 		if err := item.Validate(); err != nil {
// 			continue
// 		}
// 		out = append(out, item)
// 	}
// 	return out, nil
// }
