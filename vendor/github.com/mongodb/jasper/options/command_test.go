package options

import (
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testPostHook struct {
	count    int
	abort    bool
	errsSeen int
}

func (t *testPostHook) hook(err error) error {
	t.count++

	if err != nil {
		t.errsSeen++
		if t.abort {
			return err
		}
	}
	return nil
}

func TestCommand(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("InvalidByDefault", func(t *testing.T) {
			require.Error(t, (&Command{}).Validate())
		})
		t.Run("ValidatePopulatesProcessArgs", func(t *testing.T) {
			opts := &Command{}
			assert.Nil(t, opts.Process.Args)
			require.Error(t, opts.Validate())
			assert.NotNil(t, opts.Process.Args)
			require.Len(t, opts.Process.Args, 1)
			require.Equal(t, "", opts.Process.Args[0])
		})
		t.Run("InvalidPriorityCausesError", func(t *testing.T) {
			opts := &Command{
				Priority: -1,
				Commands: [][]string{{""}},
			}
			assert.Error(t, opts.Validate())
		})
		t.Run("Valid", func(t *testing.T) {
			opts := &Command{
				Priority: level.Info,
				Commands: [][]string{{""}},
			}
			assert.NoError(t, opts.Validate())
		})
	})
	t.Run("LoggingPreHook", func(t *testing.T) {
		sender, err := send.NewInternalLogger("pre-hook", send.LevelInfo{Default: level.Debug, Threshold: level.Debug})
		require.NoError(t, err)
		logger := logging.MakeGrip(sender)
		hook := NewLoggingPreHook(logger, level.Info)
		assert.NotNil(t, hook)
		cmd := &Command{ID: "TEST"}
		assert.False(t, sender.HasMessage())
		hook(cmd, &Create{})
		require.True(t, sender.HasMessage())
		assert.Equal(t, 1, sender.Len())
		msg, ok := sender.GetMessage().Message.Raw().(message.Fields)
		require.True(t, ok)
		assert.Equal(t, cmd.ID, msg["id"])
	})
	t.Run("PrehookConstrcutors", func(t *testing.T) {
		assert.NotNil(t, NewDefaultLoggingPreHook(level.Info))
		assert.NotNil(t, NewLoggingPreHookFromSender(grip.GetSender(), level.Debug))
	})
	t.Run("MergePreook", func(t *testing.T) {
		sender, err := send.NewInternalLogger("pre-hook", send.LevelInfo{Default: level.Debug, Threshold: level.Debug})
		require.NoError(t, err)
		logger := logging.MakeGrip(sender)

		hook := MergePreHooks(NewLoggingPreHook(logger, level.Info), NewLoggingPreHook(logger, level.Info), NewLoggingPreHook(logger, level.Info))
		assert.Equal(t, 0, sender.Len())
		hook(&Command{}, &Create{})
		assert.Equal(t, 3, sender.Len())
	})
	t.Run("MergePostHook", func(t *testing.T) {
		t.Run("Harness", func(t *testing.T) {
			t.Run("Counter", func(t *testing.T) {
				mock := &testPostHook{}
				assert.Equal(t, 0, mock.count)
				assert.NoError(t, mock.hook(nil))
				assert.Equal(t, 1, mock.count)
			})
			t.Run("Abort", func(t *testing.T) {
				mock := &testPostHook{abort: true}
				assert.NoError(t, mock.hook(nil))
				assert.Equal(t, 1, mock.count)
				assert.Equal(t, 0, mock.errsSeen)

				assert.Error(t, mock.hook(errors.New("hi")))
				assert.Equal(t, 2, mock.count)
				assert.Equal(t, 1, mock.errsSeen)
			})
		})
		t.Run("Merged", func(t *testing.T) {
			mock := &testPostHook{abort: true}
			hook := MergePostHooks(mock.hook, mock.hook)
			assert.NoError(t, hook(nil))
			assert.Equal(t, 2, mock.count)
			assert.Error(t, hook(errors.New("hi")))
			assert.Equal(t, 4, mock.count)
			assert.Equal(t, 2, mock.errsSeen)
		})
		t.Run("ShortCircuit", func(t *testing.T) {
			mock := &testPostHook{abort: true}
			hook := MergeAbortingPostHooks(mock.hook, mock.hook)
			assert.NoError(t, hook(nil))
			assert.Equal(t, 2, mock.count)

			assert.Error(t, hook(errors.New("hi")))
			assert.Equal(t, 3, mock.count)
			assert.Equal(t, 1, mock.errsSeen)
		})
		t.Run("Passthrough", func(t *testing.T) {
			mock := &testPostHook{}
			hook := MergeAbortingPassthroughPostHooks(mock.hook, mock.hook)
			assert.NoError(t, hook(nil))
			assert.Equal(t, 2, mock.count)
			assert.Equal(t, 0, mock.errsSeen)

			err := errors.New("hi")
			assert.Equal(t, err, hook(err))
			assert.Equal(t, 4, mock.count)
			assert.Equal(t, 2, mock.errsSeen)

			mock.abort = true
			assert.Equal(t, err, hook(err))
			assert.Equal(t, 5, mock.count)
			assert.Equal(t, 3, mock.errsSeen)
		})
	})
}
