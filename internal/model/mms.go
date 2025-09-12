package model

import (
	"main/internal/validateStruct"
)

// структура для MMSData
type MMSData struct {
	Country      string `json:"country" validate:"iso3166_1_alpha2"`
	Provider     string `json:"provider" validate:"oneof=Topolo Rond Kildy"`
	Bandwidth    string `json:"bandwidth" validate:"required,num0to100"`
	ResponseTime string `json:"response_time" validate:"required,number"`
}

func (v MMSData) Validate() error {
	return validateStruct.Struct(v)
}
