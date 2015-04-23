package profitbricks

import (
	"encoding/xml"
	"strings"
)

func marshalMultiRequest(name string, requests ...interface{}) string {
	lines := make([]string, 0, len(requests))
	for _, req := range requests {
		b, e := xml.Marshal(req)
		if e != nil {
			panic(e.Error())
		}
		lines = append(lines, string(b))
	}
	return "<tns:" + name + ">" + strings.Join(lines, "\n") + "</tns:" + name + ">"
}
