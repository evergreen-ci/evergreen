package birch

import "fmt"

func bestStringAttempt(in interface{}) string {
	switch val := in.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	case error:
		return val.Error()
	default:
		return fmt.Sprint(in)
	}
}
