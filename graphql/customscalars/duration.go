package customscalars

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
)

func MarshalDuration(b time.Duration) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		num := int64(b / time.Millisecond)
		w.Write([]byte(strconv.FormatInt(num, 10)))
	})
}

func UnmarshalDuration(v interface{}) (time.Duration, error) {
	switch v := v.(type) {
	case int:
		return time.Duration(v) * time.Millisecond, nil
	default:
		return time.Duration(0), fmt.Errorf("%T is not a time.Duration", v)
	}
}
