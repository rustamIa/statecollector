package model

import (
	"main/internal/validateStruct"
)

type SupportData struct {
	Topic         string `json:"topic"       validate:"required"`
	ActiveTickets int    `json:"active_tickets" validate:"gte=-1"`
}

func (v SupportData) Validate() error {
	return validateStruct.Struct(v)
}
