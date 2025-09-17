package model

import (
	valid "main/internal/validatestruct"
)

// структура для SMSData
type SMSData struct {
	Country      string `validate:"iso3166_1_alpha2"`
	Bandwidth    string `validate:"required,num0to100"` // ← только цифры (0..100)
	ResponseTime string `validate:"required,number"`    // ← в том числе float
	Provider     string `validate:"oneof=Topolo Rond Kildy"`
}

// Вызов метода валидации структуры
func (v SMSData) Validate() error {
	return valid.Struct(v)
}
