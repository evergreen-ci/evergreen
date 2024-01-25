package client

import (
	"fmt"
	"strings"

	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

const redactedVariableTemplate = "<redacted:%s>"

type redactingSender struct {
	send.Sender
	replacer *strings.Replacer
}

func (r *redactingSender) Send(m message.Composer) {
	// messageString := c.String()
	// for key, val := range vars {
	// 	messageString = strings.ReplaceAll(messageString, val, fmt.Sprintf(redactedVariableTemplate, key))
	// }
	// return send.MakeDefaultFormatter()(message.NewDefaultMessage(c.Priority(), messageString))
	r.Sender.Send(message.NewDefaultMessage(m.Priority(), r.replacer.Replace(m.String())))
}

func newRedactingSender(sender send.Sender, substitutions map[string]string) send.Sender {
	replacements := make([]string, 0, len(substitutions))
	for key, val := range substitutions {
		replacements = append(replacements, val, fmt.Sprintf(redactedVariableTemplate, key))
	}
	replacer := strings.NewReplacer(replacements...)
	return &redactingSender{
		Sender:   sender,
		replacer: replacer,
	}
}
