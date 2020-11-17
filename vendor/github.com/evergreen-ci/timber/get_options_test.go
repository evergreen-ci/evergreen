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
			name: "BaseURLMissing",
			opts: GetOptions{
				ID: "id",
			},
			hasErr: true,
		},
		{
			name: "IDandTaskIDMissing",
			opts: GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			hasErr: true,
		},
		{
			name: "IDandTaskIDPopulated",
			opts: GetOptions{
				BaseURL: "https://cedar.mongodb.com",
				ID:      "id",
				TaskID:  "task_id",
			},
			hasErr: true,
		},
		{
			name: "IDPopualted",
			opts: GetOptions{
				BaseURL: "https://cedar.mongodb.com",
				ID:      "id",
			},
		},
		{
			name: "TaskIDPopualted",
			opts: GetOptions{
				BaseURL: "https://cedar.mongodb.com",
				TaskID:  "task_id",
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
