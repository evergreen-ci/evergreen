package model

import "time"

type MyType struct {
	StringField        string    `json:"string_field"`
	AnotherStringField string    `json:"another_string_field"`
	ATime              time.Time `json:"a_time"`
	Number             int       `json:"number"`
	Complex            What      `json:"complex"`
}
type What struct {
	Yes string `json:"yes"`
}
