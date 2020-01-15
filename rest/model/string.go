package model

func ToStringPtr(in string) *string {
	return &in
}

func FromStringPtr(in *string) string {
	if in == nil {
		return ""
	}
	return *in
}

func ToStringPtrSlice(in []string) []*string {
	res := []*string{}
	for _, each := range in {
		res = append(res, ToStringPtr(each))
	}
	return res
}

func FromStringPtrSlice(in []*string) []string {
	res := []string{}
	for _, each := range in {
		res = append(res, FromStringPtr(each))
	}
	return res
}
