// Code generated by legacy codegen tool.
// Manual edits are allowed but please do not replicate this code style.

package model

func ArrstringArrstring(t []string) []string {
	m := []string{}
	for _, e := range t {
		m = append(m, StringString(e))
	}
	return m
}

func BoolBool(in bool) bool {
	return bool(in)
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
