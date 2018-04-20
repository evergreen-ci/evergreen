package model

type APIString *string

func ToAPIString(in string) APIString {
	return APIString(&in)
}

func FromAPIString(in APIString) string {
	if in == nil {
		return ""
	}
	return *in
}
