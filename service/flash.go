package service

import (
	"encoding/gob"
)

type flashMessage struct {
	Severity string
	Message  string
}

func init() {
	gob.Register(&flashMessage{})
}
