package util

func IsPtrSetToTrue(ptr *bool) bool {
	return ptr != nil && *ptr
}

func IsPtrSetToFalse(ptr *bool) bool {
	return ptr != nil && !*ptr
}
