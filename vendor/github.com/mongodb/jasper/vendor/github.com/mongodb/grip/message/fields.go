package message

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mongodb/grip/level"
)

// FieldsMsgName is the name of the default "message" field in the
// fields structure.
const FieldsMsgName = "message"

type fieldMessage struct {
	message      string
	fields       Fields
	cachedOutput string
	skipMetadata bool
	Base
}

// Fields is a convince type that wraps map[string]interface{} and is
// used for attaching structured metadata to a build request. For
// example:
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

// MakeFieldsMessage constructs a fields Composer from a message string and
// Fields object, without specifying the priority of the message.
func MakeFieldsMessage(message string, f Fields) Composer {
	m := &fieldMessage{message: message, fields: f}
	m.setup()

	return m
}

// MakeSimpleFields returns a structured Composer that does
// not attach basic logging metadata.
func MakeSimpleFields(f Fields) Composer {
	m := &fieldMessage{fields: f, skipMetadata: true}
	m.setup()
	return m
}

// NewSimpleFields returns a structured Composer that does not
// attach basic logging metadata and allows callers to configure the
// messages' log level.
func NewSimpleFields(p level.Priority, f Fields) Composer {
	m := MakeSimpleFields(f)
	_ = m.SetPriority(p)
	return m
}

// MakeSimpleFieldsMessage returns a structured Composer that does not attach
// basic logging metadata, but allows callers to specify the message
// (the "message" field) as a string.
func MakeSimpleFieldsMessage(msg string, f Fields) Composer {
	m := &fieldMessage{
		message:      msg,
		fields:       f,
		skipMetadata: true,
	}

	m.setup()
	return m
}

// NewSimpleFieldsMessage returns a structured Composer that does not attach
// basic logging metadata, but allows callers to specify the message
// (the "message" field) as well as the message's log-level.
func NewSimpleFieldsMessage(p level.Priority, msg string, f Fields) Composer {
	m := MakeSimpleFieldsMessage(msg, f)
	_ = m.SetPriority(p)
	return m
}

////////////////////////////////////////////////////////////////////////
//
// Implementation
//
////////////////////////////////////////////////////////////////////////

func (m *fieldMessage) setup() {
	if _, ok := m.fields[FieldsMsgName]; !ok && m.message != "" {
		m.fields[FieldsMsgName] = m.message
	}

	if m.skipMetadata {
		return
	}

	_ = m.Collect()

	if b, ok := m.fields["metadata"]; !ok {
		m.fields["metadata"] = &m.Base
	} else if _, ok = b.(*Base); ok {
		m.fields["metadata"] = &m.Base
	}
}

// MakeFields creates a composer interface from *just* a Fields instance.
func MakeFields(f Fields) Composer {
	m := &fieldMessage{fields: f}
	m.setup()
	return m
}

func (m *fieldMessage) Loggable() bool {
	if m.message == "" && len(m.fields) == 0 {
		return false
	}

	if len(m.fields) == 1 {
		if _, ok := m.fields["metadata"]; ok {
			return false
		}
	}

	return true
}

func (m *fieldMessage) String() string {
	if !m.Loggable() {
		return ""
	}

	if m.cachedOutput == "" {
		const tmpl = "%s='%v'"
		out := []string{}
		if m.message != "" {
			out = append(out, fmt.Sprintf(tmpl, FieldsMsgName, m.message))
		}

		for k, v := range m.fields {
			if k == FieldsMsgName && v == m.message {
				continue
			}
			if k == "time" {
				continue
			}
			if k == "metadata" {
				continue
			}

			out = append(out, fmt.Sprintf(tmpl, k, v))
		}

		sort.Sort(sort.StringSlice(out))

		m.cachedOutput = fmt.Sprintf("[%s]", strings.Join(out, " "))
	}

	return m.cachedOutput
}

func (m *fieldMessage) Raw() interface{} { return m.fields }

func (m *fieldMessage) Annotate(key string, value interface{}) error {
	if _, ok := m.fields[key]; ok {
		return fmt.Errorf("key '%s' already exists", key)
	}

	m.fields[key] = value

	return nil
}
