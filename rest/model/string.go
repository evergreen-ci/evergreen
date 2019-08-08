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

func ToAPIStringList(in []string) []APIString {
	res := []APIString{}
	for _, each := range in {
		res = append(res, ToAPIString(each))
	}
	return res
}

func FromAPIStringList(in []APIString) []string {
	res := []string{}
	for _, each := range in {
		res = append(res, FromAPIString(each))
	}
	return res
}
