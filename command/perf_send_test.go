package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPerfSendParseParams(t *testing.T) {
	for _, test := range []struct {
		name   string
		params map[string]interface{}
		hasErr bool
	}{
		{
			name:   "MissingFile",
			params: map[string]interface{}{},
			hasErr: true,
		},
		{
			name:   "FileOnly",
			params: map[string]interface{}{"file": "fn"},
			hasErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := &perfSend{}
			err := cmd.ParseParams(test.params)

			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
