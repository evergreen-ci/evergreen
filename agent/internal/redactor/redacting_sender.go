package redactor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/util"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

const redactedVariableTemplate = "<REDACTED:%s>"

// redactingSender wraps a sender for redacting sensitive expansion values and
// other Evergreen-internal values.
type redactingSender struct {
	expansions         *util.DynamicExpansions
	expansionsToRedact []string
	internalRedactions *util.DynamicExpansions
	allRedacted        []util.RedactInfo

	send.Sender
}

// RedactionOptions configures a redacting sender.
type RedactionOptions struct {
	// Expansions defines the values to redact.
	Expansions *util.DynamicExpansions
	// Redacted specifies the names of expansions to redact the values for.
	// [globals.ExpansionsToRedact] are always redacted.
	Redacted []string
	// InternalRedactions specifies an additional set of strings that are not
	// expansions that should be redacted from the logs (e.g. agent-internal
	// secrets). All values in InternalRedactions are assumed to be
	// sensitive and are replaced by their key.
	InternalRedactions *util.DynamicExpansions
	// PreloadRedactions indicates whether the redacting sender will need to compute a
	// full list of redacted key-value pairs on initialization, or on every
	// sender.Send invocation. This should not be set if the redacting sender
	// is logging during task runtime, because expansions can change over the course
	// of task runtime. It can only be safely set if  we're confident that expansions
	// will no longer be changing. This flag exists to improve the performance of test log ingestion.
	PreloadRedactions bool
}

func (r *redactingSender) Send(m message.Composer) {
	if !m.Loggable() {
		return
	}

	msg := m.String()

	var allRedacted []util.RedactInfo
	if len(r.allRedacted) > 0 {
		allRedacted = r.allRedacted
	} else {
		allRedacted = getAllRedacted(r.expansions, r.internalRedactions, r.expansionsToRedact)
	}

	for _, info := range allRedacted {
		msg = strings.ReplaceAll(msg, info.Value, fmt.Sprintf(redactedVariableTemplate, info.Key))
	}

	r.Sender.Send(message.NewDefaultMessage(m.Priority(), msg))
}

// NewRedactingSender wraps the provided sender with a sender that redacts
// expansions in accordance with the reaction options.
func NewRedactingSender(sender send.Sender, opts RedactionOptions) send.Sender {
	if opts.Expansions == nil {
		opts.Expansions = &util.DynamicExpansions{}
	}
	if opts.InternalRedactions == nil {
		opts.InternalRedactions = &util.DynamicExpansions{}
	}

	redacter := &redactingSender{
		expansions:         opts.Expansions,
		expansionsToRedact: append(opts.Redacted, globals.ExpansionsToRedact...),
		internalRedactions: opts.InternalRedactions,
		Sender:             sender,
	}
	if opts.PreloadRedactions {
		redacter.allRedacted = getAllRedacted(redacter.expansions, redacter.internalRedactions, redacter.expansionsToRedact)
	}
	return redacter
}

func getAllRedacted(expansions, internalRedactions *util.DynamicExpansions, expansionsToRedact []string) []util.RedactInfo {
	allRedacted := expansions.GetRedacted()
	for _, expansion := range expansionsToRedact {
		if val := expansions.Get(expansion); val != "" {
			allRedacted = append(allRedacted, util.RedactInfo{Key: expansion, Value: val})
		}
	}
	internalRedactions.Range(func(k, v string) bool {
		allRedacted = append(allRedacted, util.RedactInfo{Key: k, Value: v})
		return true
	})

	// Sort redacted info based on value length to ensure we're redacting longer values first.
	sort.Slice(allRedacted, func(i, j int) bool {
		return len(allRedacted[i].Value) > len(allRedacted[j].Value)
	})
	return allRedacted
}
