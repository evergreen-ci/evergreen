package recovery

import (
	"errors"
	"fmt"
)

func panicString(p interface{}) string {
	switch panicMesg := p.(type) {
	case string:
		return panicMesg
	case error:
		return panicMesg.Error()
	case fmt.Stringer:
		return panicMesg.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%+v", panicMesg)
	}
}

func panicError(p interface{}) error {
	if p == nil {
		return nil
	}

	ps := panicString(p)
	if ps == "" {
		ps = fmt.Sprintf("non-nil panic [%T] encountered with no string representation", p)
	}

	return errors.New(ps)
}
