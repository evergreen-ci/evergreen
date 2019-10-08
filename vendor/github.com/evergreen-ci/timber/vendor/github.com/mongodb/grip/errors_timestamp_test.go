package grip

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimestampError(t *testing.T) {
	t.Run("ErrorFinder", func(t *testing.T) {
		ts, ok := ErrorTimeFinder(nil)
		assert.False(t, ok)
		assert.Zero(t, ts)

		ts, ok = ErrorTimeFinder(newTimeStampError(nil))
		assert.False(t, ok)
		assert.Zero(t, ts)

		ts, ok = ErrorTimeFinder(newTimeStampError(errors.New("hello")))
		assert.True(t, ok)
		assert.NotZero(t, ts)

		ts, ok = ErrorTimeFinder(errors.Wrap(errors.WithStack((errors.New("hello"))), "world"))
		assert.False(t, ok)
		assert.Zero(t, ts)

		ts, ok = ErrorTimeFinder(errors.WithStack(newTimeStampError(errors.New("hello"))))
		assert.True(t, ok)
		assert.NotZero(t, ts)

		ts, ok = ErrorTimeFinder(errors.WithStack(newTimeStampError(nil)))
		assert.False(t, ok)
		assert.Zero(t, ts)

		ts, ok = ErrorTimeFinder(newTimeStampError(errors.WithStack(errors.New("hello"))))
		assert.True(t, ok)
		assert.NotZero(t, ts)

		ts, ok = ErrorTimeFinder(errors.Wrap(errors.WithStack(newTimeStampError(errors.New("hello"))), "world"))
		assert.True(t, ok)
		assert.NotZero(t, ts)
	})
	t.Run("Wrap", func(t *testing.T) {
		t.Run("WithTimestamp", func(t *testing.T) {
			ts, ok := ErrorTimeFinder(nil)
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTime(nil))
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTime(errors.New("hello")))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.WithStack(WrapErrorTime(errors.New("hello"))))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.WithStack(WrapErrorTime(nil)))
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(errors.Wrap(errors.WithStack(WrapErrorTime(errors.New("hello"))), "world"))
			assert.True(t, ok)
			assert.NotZero(t, ts)
		})
		t.Run("WithTimestampMessage", func(t *testing.T) {
			ts, ok := ErrorTimeFinder(nil)
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTimeMessage(nil, "earth"))
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTimeMessage(errors.New("hello"), "earth"))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.WithStack(WrapErrorTimeMessage(errors.New("hello"), "earth")))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.WithStack(WrapErrorTimeMessage(nil, "earth")))
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTimeMessage(errors.WithStack(errors.New("hello")), "earth"))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.Wrap(errors.WithStack(WrapErrorTimeMessage(errors.New("hello"), "earth")), "world"))
			assert.True(t, ok)
			assert.NotZero(t, ts)
		})
		t.Run("WithTimestampMessageFormatEmpty", func(t *testing.T) {
			ts, ok := ErrorTimeFinder(nil)
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTimeMessagef(nil, "earth"))
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTimeMessagef(errors.New("hello"), "earth"))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.WithStack(WrapErrorTimeMessagef(errors.New("hello"), "earth")))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.WithStack(WrapErrorTimeMessagef(nil, "earth")))
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTimeMessagef(errors.WithStack(errors.New("hello")), "earth"))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.Wrap(errors.WithStack(WrapErrorTimeMessagef(errors.New("hello"), "earth")), "world"))
			assert.True(t, ok)
			assert.NotZero(t, ts)
		})
		t.Run("WrapWithTimestampMessageFormat", func(t *testing.T) {
			ts, ok := ErrorTimeFinder(nil)
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTimeMessagef(nil, "earth-%s", "lings"))
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTimeMessagef(errors.New("hello"), "earth-%s", "lings"))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.WithStack(WrapErrorTimeMessagef(errors.New("hello"), "earth-%s", "lings")))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.WithStack(WrapErrorTimeMessagef(nil, "earth-%s", "lings")))
			assert.False(t, ok)
			assert.Zero(t, ts)

			ts, ok = ErrorTimeFinder(WrapErrorTimeMessagef(errors.WithStack(errors.New("hello")), "earth-%s", "lings"))
			assert.True(t, ok)
			assert.NotZero(t, ts)

			ts, ok = ErrorTimeFinder(errors.Wrap(errors.WithStack(WrapErrorTimeMessagef(errors.New("hello"), "earth-%s", "lings")), "world"))
			assert.True(t, ok)
			assert.NotZero(t, ts)
		})
	})
	t.Run("Interfaces", func(t *testing.T) {
		t.Run("Cause", func(t *testing.T) {
			we := WrapErrorTime(errors.New("hello"))
			assert.Equal(t, "hello", we.(errorCauser).Cause().Error())
		})
		t.Run("FormattingBasic", func(t *testing.T) {
			err := &timestampError{
				time: time.Now(),
				err:  errors.New("hello world"),
			}
			assert.Contains(t, err.Error(), err.time.Format(time.RFC3339))
			assert.Contains(t, err.Error(), "hello world")
		})
		t.Run("FormattingStrong", func(t *testing.T) {
			err := &timestampError{
				time: time.Now(),
				err:  errors.New("hello world"),
			}
			assert.Contains(t, fmt.Sprintf("%+v", err), err.time.Format(time.RFC3339))
			assert.Contains(t, fmt.Sprintf("%+v", err), "hello world")
			assert.Contains(t, fmt.Sprintf("%q", err), err.time.Format(time.RFC3339))
			assert.Contains(t, fmt.Sprintf("%q", err), "hello world")
			assert.Contains(t, fmt.Sprintf("%s", err), err.time.Format(time.RFC3339)) // nolint
			assert.Contains(t, fmt.Sprintf("%s", err), "hello world")                 // nolint
		})
		t.Run("Composer", func(t *testing.T) {
			assert.Implements(t, (*message.Composer)(nil), &timestampError{})
			t.Run("Nil", func(t *testing.T) {
				err := &timestampError{}
				assert.False(t, err.Loggable())
				assert.Zero(t, err.String())
				assert.NoError(t, err.Annotate("foo", "bar"))
			})
			t.Run("Loggable", func(t *testing.T) {
				msg, ok := WrapErrorTime(errors.New("hello world")).(message.Composer)
				require.True(t, ok)

				assert.True(t, msg.Loggable())
				assert.Equal(t, "hello world", msg.String())
			})
			t.Run("Annotate", func(t *testing.T) {
				msg, ok := WrapErrorTime(errors.New("hello world")).(message.Composer)
				require.True(t, ok)
				assert.NoError(t, msg.Annotate("foo", "bar"))
				assert.Error(t, msg.Annotate("foo", "bar"))
				assert.NoError(t, msg.Annotate("bar", "bar"))
			})
			t.Run("RawJSON", func(t *testing.T) {
				msg, ok := WrapErrorTime(errors.New("hello world")).(message.Composer)
				require.True(t, ok)

				for i := 0; i < 10; i++ {
					_, err := json.Marshal(msg.Raw())
					require.NoError(t, err)
				}
			})
			t.Run("Levels", func(t *testing.T) {
				err := &timestampError{}
				require.Error(t, err.SetPriority(1000))
				require.NoError(t, err.SetPriority(level.Debug))
				require.Equal(t, level.Debug, err.Priority())

				require.Error(t, err.SetPriority(1000))
				require.Equal(t, level.Debug, err.Priority())

				require.NoError(t, err.SetPriority(level.Alert))
				require.Equal(t, level.Alert, err.Priority())
			})
		})
	})
}
