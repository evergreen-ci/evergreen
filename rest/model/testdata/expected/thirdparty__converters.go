// Code generated by rest/model/codegen.go. DO NOT EDIT.

package model

func StringString(in string) string {
	return string(in)
}

func StringStringPtr(in string) *string {
	out := string(in)
	return &out
}

func StringPtrString(in *string) string {
	var out string
	if in == nil {
		return out
	}
	return string(*in)
}

func StringPtrStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	out := string(*in)
	return &out
}
