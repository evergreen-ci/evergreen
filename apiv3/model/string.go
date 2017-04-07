package model

import (
	"encoding/json"
)

type APIString string

func (as *APIString) MarshalJSON() ([]byte, error) {
	if *as == "" {
		return []byte("null"), nil
	}
	return json.Marshal(as)
}

func (as *APIString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, (*string)(as))
}
