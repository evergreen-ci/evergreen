package message

import (
	"fmt"
)

type errorComposerWrap struct {
	err    error
	cached string
	Composer
}

// NewErrorWrappedComposer provvides a way to construct a log message
// that annotates an error.
func NewErrorWrappedComposer(err error, m Composer) Composer {
	return &errorComposerWrap{
		err:      err,
		Composer: m,
	}
}

func (m *errorComposerWrap) String() string {
	if m.cached == "" {
		m.cached = fmt.Sprintf("%s: %v", m.Composer.String(), m.err.Error())
	}

	return m.cached
}

func (m *errorComposerWrap) Loggable() bool {
	return m.err != nil
}

func (m *errorComposerWrap) Raw() interface{} {
	return Fields{
		"error":    m.err.Error(),
		"extended": fmt.Sprintf("%+v", m.err),
		"payload":  m.Composer.Raw(),
	}
}
