// Code generated by rest/model/codegen.go. DO NOT EDIT.

package model

func IntInt(in int) int {
	return int(in)
}

func IntIntPtr(in int) *int {
	out := int(in)
	return &out
}

func IntPtrInt(in *int) int {
	var out int
	if in == nil {
		return out
	}
	return int(*in)
}

func IntPtrIntPtr(in *int) *int {
	if in == nil {
		return nil
	}
	out := int(*in)
	return &out
}

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
