package model

import (
	"main/internal/validateStruct"
)

type EmailData struct {
	Country      string `validate:"iso3166_1_alpha2"`
	Provider     string `validate:"oneof=Gmail Yahoo Hotmail MSN Orange Comcast AOL Live RediffMail GMX Protonmail Yandex Mail.ru"`
	DeliveryTime int    `validate:"required"`
}

// Вызов метода валидации структуры
func (v EmailData) Validate() error {
	return validateStruct.Struct(v)
}
