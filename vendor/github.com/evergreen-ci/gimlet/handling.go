package gimlet

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func writeResponse(of OutputFormat, w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", of.ContentType())

	w.WriteHeader(code)

	size, err := writePayload(w, data)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		grip.Warningf("encountered error '%s' writing a %d (%s) response ", err.Error(), size, of)
	}
}

func writePayload(w io.Writer, data interface{}) (int, error) {
	switch data := data.(type) {
	case []byte:
		return w.Write(data)
	case string:
		return w.Write([]byte(data))
	case error:
		return w.Write([]byte(data.Error()))
	case []string:
		return w.Write([]byte(strings.Join(data, "\n")))
	case *bytes.Buffer:
		return w.Write(data.Bytes())
	case fmt.Stringer:
		return w.Write([]byte(data.String()))
	case io.Reader:
		size, err := io.Copy(w, data)
		return int(size), errors.WithStack(err)
	case []io.Reader:
		size, err := io.Copy(w, io.MultiReader(data...))
		return int(size), errors.WithStack(err)
	default:
		return w.Write([]byte(fmt.Sprintf("%v", data)))
	}
}
