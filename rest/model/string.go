package model

type APIString *string

func ToApiString(in string) APIString {
	return APIString(&in)
}

func FromApiString(in APIString) string {
	if in == nil {
		return ""
	}
	return *in
}
