package send

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/mongodb/grip/message"
)

const (
	defaultFormatTmpl  = "[p=%s]: %s"
	callSiteTmpl       = "[p=%s] [%s:%d]: %s"
	completeFormatTmpl = "[%s] (p=%s) %s"
)

type MessageFormatter func(message.Composer) (string, error)

func MakeJSONFormatter() MessageFormatter {
	return func(m message.Composer) (string, error) {
		out, err := json.Marshal(m.Raw())
		if err != nil {
			return "", err
		}

		return string(out), nil
	}
}

func MakeDefaultFormatter() MessageFormatter {
	return func(m message.Composer) (string, error) {
		return fmt.Sprintf(defaultFormatTmpl, m.Priority(), m.String()), nil
	}
}

func MakePlainFormatter() MessageFormatter {
	return func(m message.Composer) (string, error) {
		return m.String(), nil
	}
}

func MakeCallSiteFormatter(depth int) MessageFormatter {
	depth++
	return func(m message.Composer) (string, error) {
		file, line := callerInfo(depth)
		return fmt.Sprintf(callSiteTmpl, m.Priority(), file, line, m), nil
	}
}

func MakeXMPPFormatter(name string) MessageFormatter {
	return func(m message.Composer) (string, error) {
		return fmt.Sprintf(completeFormatTmpl, name, m.Priority(), m.String()), nil
	}
}

func callerInfo(depth int) (string, int) {
	// increase depth to account for callerInfo itself.
	depth++

	// get caller info.
	_, file, line, _ := runtime.Caller(depth)

	// get the directory and filename
	dir, fileName := filepath.Split(file)
	file = filepath.Join(filepath.Base(dir), fileName)

	return file, line
}
