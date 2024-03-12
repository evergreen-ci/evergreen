package redactor

import (
	"fmt"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/util"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

const redactedVariableTemplate = "<REDACTED:%s>"

// redactingSender wraps a sender for redacting sensitive expansion values.
type redactingSender struct {
	expansions         *util.DynamicExpansions
	expansionsToRedact []string

	send.Sender
}

type RedactionOptions struct {
	// Expansions defines the values to redact.
	Expansions *util.DynamicExpansions
	// Redacted specifies the names of expansions to redact the values for.
	Redacted []string
}

func (r *redactingSender) Send(m message.Composer) {
	if !m.Loggable() {
		return
	}

	msg := m.String()
	for _, expansion := range r.expansionsToRedact {
		if val := r.expansions.Get(expansion); val != "" {
			msg = strings.ReplaceAll(msg, val, fmt.Sprintf(redactedVariableTemplate, expansion))
		}
	}
	r.Sender.Send(message.NewDefaultMessage(m.Priority(), msg))
}

func NewRedactingSender(sender send.Sender, opts RedactionOptions) send.Sender {
	return &redactingSender{
		expansions:         opts.Expansions,
		expansionsToRedact: opts.Redacted,
		Sender:             sender,
	}
}
