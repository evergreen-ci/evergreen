package recovery

import "fmt"

func panicString(p interface{}) string {
	panicMsg, ok := p.(string)
	if !ok {
		panicMsg = fmt.Sprintf("%+v", panicMsg)
	}

	return panicMsg
}
