package taskoutput

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, test := range []struct {
		name     string
		handlers []*mockDirectoryHandler
		hasErr   bool
	}{
		{
			name: "RunWithHandlerError",
			handlers: []*mockDirectoryHandler{
				{},
				{},
				{runErr: true},
			},
			hasErr: true,
		},
		{
			name: "Run",
			handlers: []*mockDirectoryHandler{
				{},
				{},
				{},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := &Directory{
				root:     utility.RandomString(),
				handlers: map[string]directoryHandler{},
			}
			for _, handler := range test.handlers {
				d.handlers[filepath.Join(d.root, utility.RandomString())] = handler
			}

			require.NoError(t, d.Setup())
			for dir := range d.handlers {
				_, err := os.Stat(dir)
				assert.NoError(t, err)
			}

			err := d.Run(ctx)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			for _, handler := range test.handlers {
				assert.True(t, handler.ran)
			}
			_, err = os.Stat(d.root)
			assert.Error(t, err)
		})
	}
}

type mockDirectoryHandler struct {
	runErr bool
	ran    bool
}

func (m *mockDirectoryHandler) run(_ context.Context) error {
	if m.ran {
		return errors.New("already called run")
	}
	m.ran = true

	if m.runErr {
		return errors.New("run error")
	}

	return nil
}
