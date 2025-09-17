package model

import (
	valid "main/internal/validatestruct"
)

type IncidentData struct {
	Topic  string `json:"topic" validate:"required"`
	Status string `json:"status" validate:"oneof=active closed"`
}

func (v IncidentData) Validate() error {
	return valid.Struct(v)
}
