package taskoutput

import (
	"context"
	"os"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestDirectory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, test := range []struct {
		name     string
		handlers []*mockDirectoryHandler
		hasErr   bool
	}{
		{
			name: "StartAndCloseWithHandlerError",
			handlers: []*mockDirectoryHandler{
				&mockDirectoryHandler{},
				&mockDirectoryHandler{},
				&mockDirectoryHandler{startErr: true, closeErr: true},
			},
			hasErr: true,
		},
		{
			name: "StartAndClose",
			handlers: []*mockDirectoryHandler{
				&mockDirectoryHandler{},
				&mockDirectoryHandler{},
				&mockDirectoryHandler{},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := &Directory{
				root:     utility.RandomString(),
				handlers: map[string]directoryHandler{},
			}
			for _, handler := range test.handlers {
				d.handlers[utility.RandomString()] = handler
			}

			err := d.Start(ctx)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			for _, handler := range test.handlers {
				assert.True(t, handler.started)
				_, err = os.Stat(handler.dir)
				assert.NoError(t, err)
			}

			err = d.Close(ctx)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			_, err = os.Stat(d.root)
			assert.Error(t, err)
			for _, handler := range test.handlers {
				assert.True(t, handler.closed)
				_, err = os.Stat(handler.dir)
				assert.Error(t, err)
			}
		})
	}
}

type mockDirectoryHandler struct {
	dir      string
	startErr bool
	started  bool
	closeErr bool
	closed   bool
}

func (m *mockDirectoryHandler) start(_ context.Context, dir string) error {
	if m.started {
		return errors.New("already called start")
	}
	m.dir = dir
	m.started = true

	if m.startErr {
		return errors.New("start error")
	}
	return nil
}

func (m *mockDirectoryHandler) close(_ context.Context) error {
	m.closed = true

	if m.closeErr {
		return errors.New("start error")
	}
	return nil
}
