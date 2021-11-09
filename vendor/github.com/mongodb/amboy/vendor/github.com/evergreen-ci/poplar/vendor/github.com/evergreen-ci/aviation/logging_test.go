package aviation

import (
	"context"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestGripInterceptors(t *testing.T) {
	for _, test := range []struct {
		name       string
		unaryInfo  *grpc.UnaryServerInfo
		streamInfo *grpc.StreamServerInfo
		action     string
		err        bool
	}{
		{
			name:      "ValidLogging",
			unaryInfo: &grpc.UnaryServerInfo{},
		},
		{
			name:      "ErrorLogging",
			unaryInfo: &grpc.UnaryServerInfo{},
			action:    "error",
			err:       true,
		},
		{
			name:      "PanicLogging",
			unaryInfo: &grpc.UnaryServerInfo{},
			action:    "panic",
			err:       true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Run("Unary", func(t *testing.T) {
				sender, err := send.NewInternalLogger("test", grip.GetSender().Level())
				require.NoError(t, err)
				journaler := logging.MakeGrip(sender)
				startAt := getNumber()

				interceptor := MakeGripUnaryInterceptor(journaler)
				_, err = interceptor(context.TODO(), test.action, &grpc.UnaryServerInfo{}, mockUnaryHandler)
				if test.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}

				assert.Equal(t, startAt+2, getNumber())
				if assert.True(t, sender.HasMessage()) {
					require.Equal(t, 2, sender.Len())
					msg := sender.GetMessage()
					assert.Equal(t, level.Debug, msg.Priority)
					msg = sender.GetMessage()
					assert.Equal(t, expectedPriority(test.action), msg.Priority)
				}
			})
			t.Run("Streaming", func(t *testing.T) {
				sender, err := send.NewInternalLogger("test", grip.GetSender().Level())
				require.NoError(t, err)
				journaler := logging.MakeGrip(sender)
				startAt := getNumber()

				interceptor := MakeGripStreamInterceptor(journaler)
				err = interceptor(test.action, &mockServerStream{}, &grpc.StreamServerInfo{}, mockStreamHandler)
				if test.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}

				assert.Equal(t, startAt+2, getNumber())
				if assert.True(t, sender.HasMessage()) {
					require.Equal(t, 2, sender.Len())
					msg := sender.GetMessage()
					assert.Equal(t, level.Debug, msg.Priority)
					msg = sender.GetMessage()
					assert.Equal(t, expectedPriority(test.action), msg.Priority)
				}
			})
		})
	}
}

func expectedPriority(action string) level.Priority {
	switch action {
	case "error":
		return level.Error
	case "panic":
		return level.Alert
	default:
		return level.Debug
	}
}
