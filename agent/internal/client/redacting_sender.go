package client

import (
	"fmt"
	"strings"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

const redactedVariableTemplate = "<REDACTED:%s>"

type redactingSender struct {
	send.Sender
	expansions         util.Expansions
	expansionsToRedact []string
}

func (r *redactingSender) Send(m message.Composer) {
	msg := m.String()
	for _, expansion := range r.expansionsToRedact {
		if val := r.expansions.Get(expansion); val != "" {
			msg = strings.ReplaceAll(msg, val, fmt.Sprintf(redactedVariableTemplate, expansion))
		}
	}
	r.Sender.Send(message.NewDefaultMessage(m.Priority(), msg))
}

func newRedactingSender(sender send.Sender, expansions util.Expansions, expansionsToRedact []string) send.Sender {
	return &redactingSender{
		Sender:             sender,
		expansions:         expansions,
		expansionsToRedact: expansionsToRedact,
	}
}
