package customscalars

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func MarshalDuration(b time.Duration) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		num := int64(b / time.Millisecond)
		_, err := w.Write([]byte(strconv.FormatInt(num, 10)))
		if err != nil {
			grip.Error(errors.Wrap(err, "error marshaling Duration GQL scalar type"))
		}
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
