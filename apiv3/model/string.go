package model

import (
	"encoding/json"
)

type APIString string

func (as *APIString) MarshalJSON() ([]byte, error) {
	if *as == "" {
		return []byte("null"), nil
	}
	str := string(*as)
	return json.Marshal(&str)
}

func (as *APIString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, (*string)(as))
}
