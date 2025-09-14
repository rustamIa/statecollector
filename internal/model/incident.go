package model

import (
	"main/internal/validateStruct"
)

type IncidentData struct {
	Topic  string `json:"topic" validate:"required"`
	Status string `json:"status" validate:"oneof=active closed"`
}

func (v IncidentData) Validate() error {
	return validateStruct.Struct(v)
}
