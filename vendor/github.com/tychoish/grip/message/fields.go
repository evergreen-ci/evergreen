package message

import (
	"fmt"
	"strings"

	"github.com/tychoish/grip/level"
)

type fieldMessage struct {
	message      string
	fields       Fields
	cachedOutput string
	Base
}

// A convince type that wraps map[string]interface{} and is used for
// attaching structured metadata to a build request. For example:
//
//     message.Fields{"key0", <value>, "key1", <value>}
type Fields map[string]interface{}

// NewFieldsMessage creates a fully configured Composer instance that
// will attach some additional structured data. This constructor
// allows you to include a string message as well as Fields
// object.
func NewFieldsMessage(p level.Priority, message string, f Fields) Composer {
	m := MakeFieldsMessage(message, f)

	_ = m.SetPriority(p)

	return m
}

// NewFields constructs a full configured fields Composer.
func NewFields(p level.Priority, f Fields) Composer {
	m := MakeFields(f)
	_ = m.SetPriority(p)

	return m
}

// NewFields constructs a fields Composer from a message string and
// Fields object, without specifying the priority of the message.
func MakeFieldsMessage(message string, f Fields) Composer {
	return &fieldMessage{message: message, fields: f}
}

// MakeFields creates a composer interface from *just* a Fields instance.
func MakeFields(f Fields) Composer { return &fieldMessage{fields: f} }

func (m *fieldMessage) Loggable() bool { return m.message != "" || len(m.fields) > 0 }
func (m *fieldMessage) String() string {
	if m.cachedOutput == "" {
		const tmpl = "%s='%s'"
		out := []string{}
		if m.message != "" {
			out = append(out, fmt.Sprintf(tmpl, "msg", m.message))
		}

		for k, v := range m.fields {
			if k == "msg" && v == m.message {
				continue
			}
			if k == "time" {
				continue
			}

			out = append(out, fmt.Sprintf(tmpl, k, v))
		}

		m.cachedOutput = fmt.Sprintf("[%s]", strings.Join(out, " "))
	}

	return m.cachedOutput
}

func (m *fieldMessage) Raw() interface{} {
	_ = m.Collect()
	if _, ok := m.fields["msg"]; !ok {
		m.fields["msg"] = m.message
	}
	if _, ok := m.fields["time"]; !ok {
		m.fields["time"] = m.Time
	}

	return m.fields
}
