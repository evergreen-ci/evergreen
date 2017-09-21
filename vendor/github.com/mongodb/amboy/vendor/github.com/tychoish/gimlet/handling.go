package gimlet

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/mongodb/grip"
)

func writeResponse(of OutputFormat, w http.ResponseWriter, code int, data []byte) {
	w.Header().Set("Content-Type", of.ContentType())

	w.WriteHeader(code)

	size, err := w.Write(data)

	if err != nil {
		grip.Warningf("encountered error %s writing a %d (%s) response ", err.Error(), size, of)
	}
}

func convertToBin(data interface{}) []byte {
	switch data := data.(type) {
	case []byte:
		return data
	case string:
		return []byte(data)
	case error:
		return []byte(data.Error())
	case []string:
		var out []byte
		for _, s := range data {
			out = append(out, []byte(s)...)
		}
		return out
	case *bytes.Buffer:
		return data.Bytes()
	case fmt.Stringer:
		return []byte(data.String())
	default:
		return []byte(fmt.Sprintf("%v", data))
	}
}

func convertToBytes(data interface{}) []byte {
	switch data := data.(type) {
	case []byte:
		return data
	case string:
		return []byte(data)
	case error:
		return []byte(data.Error())
	case []string:
		return []byte(strings.Join(data, "\n"))
	case *bytes.Buffer:
		return data.Bytes()
	case fmt.Stringer:
		return []byte(data.String())
	default:
		return []byte(fmt.Sprintf("%v", data))
	}
}
