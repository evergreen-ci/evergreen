package util

func IsPtrSetToTrue(ptr *bool) bool {
	return ptr != nil && *ptr
}

func IsPtrSetToFalse(ptr *bool) bool {
	return ptr != nil && !*ptr
}

func TruePtr() *bool {
	res := true
	return &res
}

func FalsePtr() *bool {
	res := false
	return &res
}
