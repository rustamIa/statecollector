package model

import (
	valid "main/internal/validatestruct"
)

type VoiceCallData struct {
	Country             string  `validate:"iso3166_1_alpha2"`
	Bandwidth           string  `validate:"required,num0to100"` // ← только цифры (0..100)
	ResponseTime        string  `validate:"required,number"`    // ← в том числе float
	Provider            string  `validate:"oneof=TransparentCalls E-Voice JustPhone"`
	ConnectionStability float32 `validate:"required"`
	TTFB                int     `validate:"required"`
	VoicePurity         int     `validate:"required"`
	MedianOfCallsTime   int     `validate:"required"`
}

// Вызов метода валидации структуры
func (v VoiceCallData) Validate() error {
	return valid.Struct(v)
}
