package recovery

import (
	"errors"
	"fmt"
)

func panicString(p interface{}) string {
	panicMsg, ok := p.(string)
	if !ok {
		panicMsg = fmt.Sprintf("%+v", panicMsg)
	}

	return panicMsg
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
