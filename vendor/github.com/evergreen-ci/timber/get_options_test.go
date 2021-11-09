package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetOptionsValidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		opts   GetOptions
		hasErr bool
	}{
		{
			name:   "BaseURLMissing",
			opts:   GetOptions{},
			hasErr: true,
		},
		{
			name: "BaseURLPopulated",
			opts: GetOptions{
				BaseURL: "https://url.com",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.hasErr {
				assert.Error(t, test.opts.Validate())
			} else {
				assert.NoError(t, test.opts.Validate())
			}
		})
	}
}
