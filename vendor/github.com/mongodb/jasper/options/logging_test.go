package options

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogging(t *testing.T) {
	t.Run("LoggingSendErrors", func(t *testing.T) {
		lp := &LoggingPayload{}
		cl := &CachedLogger{}
		t.Run("Unconfigured", func(t *testing.T) {
			err := cl.Send(lp)
			require.Error(t, err)
			require.Equal(t, "no output configured", err.Error())
		})
		t.Run("IvalidMessage", func(t *testing.T) {
			lp.Format = LoggingPayloadFormatJSON
			lp.Data = "hello, world!"
			logger := &CachedLogger{Output: grip.GetSender()}
			require.Error(t, logger.Send(lp))
		})

	})
	t.Run("OutputTargeting", func(t *testing.T) {
		output := send.MakeInternalLogger()
		error := send.MakeInternalLogger()
		lp := &LoggingPayload{Data: "hello world!", Priority: level.Info}
		t.Run("Output", func(t *testing.T) {
			assert.Equal(t, 0, output.Len())
			cl := &CachedLogger{Output: output}
			require.NoError(t, cl.Send(lp))
			require.Equal(t, 1, output.Len())
			msg := output.GetMessage()
			assert.Equal(t, "hello world!", msg.Message.String())
		})
		t.Run("Error", func(t *testing.T) {
			assert.Equal(t, 0, error.Len())
			cl := &CachedLogger{Error: error}
			require.NoError(t, cl.Send(lp))
			require.Equal(t, 1, error.Len())
			msg := error.GetMessage()
			assert.Equal(t, "hello world!", msg.Message.String())
		})
		t.Run("ErrorForce", func(t *testing.T) {
			lp.PreferSendToError = true
			assert.Equal(t, 0, error.Len())
			cl := &CachedLogger{Error: error, Output: output}
			require.NoError(t, cl.Send(lp))
			require.Equal(t, 1, error.Len())
			msg := error.GetMessage()
			assert.Equal(t, "hello world!", msg.Message.String())
		})
	})
	t.Run("Messages", func(t *testing.T) {
		t.Run("SingleMessageProduction", func(t *testing.T) {
			t.Run("JSON", func(t *testing.T) {
				lp := &LoggingPayload{Format: LoggingPayloadFormatJSON}

				t.Run("Invalid", func(t *testing.T) {
					_, err := lp.produceMessage([]byte("hello world! 42!"))
					require.Error(t, err)
				})
				t.Run("Valid", func(t *testing.T) {
					msg, err := lp.produceMessage([]byte(`{"msg":"hello world!"}`))
					require.NoError(t, err)
					require.Equal(t, `[msg='hello world!']`, msg.String())
					raw, err := json.Marshal(msg.Raw())
					require.NoError(t, err)
					require.Len(t, raw, 22)
				})
				t.Run("ValidMetadata", func(t *testing.T) {
					lp.AddMetadata = true
					msg, err := lp.produceMessage([]byte(`{"msg":"hello world!"}`))
					require.NoError(t, err)
					require.Equal(t, `[msg='hello world!']`, msg.String())
					raw, err := json.Marshal(msg.Raw())
					require.NoError(t, err)
					require.True(t, len(raw) >= 150)
					assert.Contains(t, string(raw), "process")
					assert.Contains(t, string(raw), "hostname")
					assert.Contains(t, string(raw), "metadata")
				})
			})
			t.Run("BSON", func(t *testing.T) {
				lp := &LoggingPayload{Format: LoggingPayloadFormatBSON}
				doc := birch.DC.Elements(birch.EC.String("msg", "hello world!"))
				t.Run("Invalid", func(t *testing.T) {
					_, err := lp.produceMessage([]byte("\x01\x00"))
					require.Error(t, err)
				})
				t.Run("Valid", func(t *testing.T) {
					data, err := doc.MarshalBSON()
					require.NoError(t, err)
					msg, err := lp.produceMessage(data)
					require.NoError(t, err)

					require.Equal(t, `[msg='hello world!']`, msg.String())
					raw, err := json.Marshal(msg.Raw())
					require.NoError(t, err)
					require.Len(t, raw, 22)
				})
				t.Run("ValidMetadata", func(t *testing.T) {
					lp.AddMetadata = true
					data, err := doc.MarshalBSON()
					require.NoError(t, err)
					msg, err := lp.produceMessage(data)
					require.NoError(t, err)

					require.Equal(t, `[msg='hello world!']`, msg.String())
					raw, err := json.Marshal(msg.Raw())
					require.NoError(t, err)
					require.True(t, len(raw) >= 150)
					assert.Contains(t, string(raw), "process")
					assert.Contains(t, string(raw), "hostname")
					assert.Contains(t, string(raw), "metadata")
				})
			})
			t.Run("String", func(t *testing.T) {
				lp := &LoggingPayload{Format: LoggingPayloadFormatString}

				msg, err := lp.produceMessage([]byte("hello world! 42!"))
				require.NoError(t, err)
				require.Equal(t, "hello world! 42!", msg.String())
				t.Run("Raw", func(t *testing.T) {
					raw, err := json.Marshal(msg.Raw())
					require.NoError(t, err)
					require.True(t, len(raw) >= 24)
				})
				t.Run("WithMetadata", func(t *testing.T) {
					lp.AddMetadata = true

					msg, err := lp.produceMessage([]byte("hello world! 42!"))
					require.NoError(t, err)
					require.Equal(t, "hello world! 42!", msg.String())
					raw, err := json.Marshal(msg.Raw())
					require.NoError(t, err)
					require.True(t, len(raw) >= 50, "%d:%s", len(raw), string(raw))
				})
			})
		})
		t.Run("ConvertSingle", func(t *testing.T) {
			lp := &LoggingPayload{}
			t.Run("String", func(t *testing.T) {
				msg, err := lp.convertMessage("hello world")
				require.NoError(t, err)
				require.Equal(t, "hello world", msg.String())
			})
			t.Run("ByteSlice", func(t *testing.T) {
				msg, err := lp.convertMessage([]byte("hello world"))
				require.NoError(t, err)
				require.Equal(t, "hello world", msg.String())
			})
			t.Run("StringSlice", func(t *testing.T) {
				msg, err := lp.convertMessage([]string{"hello", "world"})
				require.NoError(t, err)
				require.Equal(t, "hello world", msg.String())
			})
			t.Run("MultiByteSlice", func(t *testing.T) {
				msg, err := lp.convertMessage([][]byte{[]byte("hello"), []byte("world")})
				require.NoError(t, err)
				require.Equal(t, "[hello world]", msg.String())
			})
			t.Run("InterfaceSlice", func(t *testing.T) {
				msg, err := lp.convertMessage([]interface{}{"hello", true, "world", 42})
				require.NoError(t, err)
				require.Equal(t, "hello true world 42", msg.String())
			})
			t.Run("Interface", func(t *testing.T) {
				msg, err := lp.convertMessage(ex{})
				require.NoError(t, err)
				require.Equal(t, "hello world!", msg.String())
			})
			t.Run("Composer", func(t *testing.T) {
				msg, err := lp.convertMessage(message.NewString("jasper"))
				require.NoError(t, err)
				require.Equal(t, "jasper", msg.String())
			})
		})
		t.Run("ConvertMulti", func(t *testing.T) {
			lp := &LoggingPayload{}
			t.Run("String", func(t *testing.T) {
				t.Run("Single", func(t *testing.T) {
					msg, err := lp.convertMultiMessage("hello world")
					require.NoError(t, err)
					require.Equal(t, "hello world", msg.String())
				})
				t.Run("Many", func(t *testing.T) {
					msg, err := lp.convertMultiMessage("hello\nworld")
					require.NoError(t, err)
					group := requireIsGroup(t, 2, msg)
					assert.Equal(t, "hello", group[0].String())
					assert.Equal(t, "world", group[1].String())
				})
			})
			t.Run("Byte", func(t *testing.T) {
				t.Run("Strings", func(t *testing.T) {
					msg, err := lp.convertMultiMessage([]byte("hello\x00world"))
					require.NoError(t, err)
					group := requireIsGroup(t, 2, msg)
					assert.Equal(t, "hello", group[0].String())
					assert.Equal(t, "world", group[1].String())

				})
				t.Run("BSON", func(t *testing.T) {
					lp.Format = LoggingPayloadFormatBSON
					defer func() { lp.Format = "" }()
					buf := &bytes.Buffer{}
					for i := 0; i < 10; i++ {
						_, err := birch.DC.Elements(
							birch.EC.String("hello", "world"),
							birch.EC.Int("idx", i),
							birch.EC.Int64("val", rand.Int63n(1+int64(i*42))),
						).WriteTo(buf)
						require.NoError(t, err)
					}

					msg, err := lp.convertMultiMessage(buf.Bytes())
					require.NoError(t, err)
					msgs := requireIsGroup(t, 10, msg)
					assert.Contains(t, msgs[0].String(), "idx='0'")
				})
			})
			t.Run("InterfaceSlice", func(t *testing.T) {
				msg, err := lp.convertMultiMessage([]interface{}{"hello", true, "world", 42})
				require.NoError(t, err)
				msgs := requireIsGroup(t, 4, msg)
				assert.Equal(t, "hello", msgs[0].String())
				assert.Equal(t, "true", msgs[1].String())
				assert.Equal(t, "42", msgs[3].String())
			})
			t.Run("Composers", func(t *testing.T) {
				msg, err := lp.convertMultiMessage([]message.Composer{
					message.NewString("hello world"),
					message.NewString("jasper"),
					message.NewString("grip"),
				})
				require.NoError(t, err)
				msgs := requireIsGroup(t, 3, msg)
				assert.Equal(t, "hello world", msgs[0].String())
				assert.Equal(t, "grip", msgs[2].String())
			})

		})
		t.Run("ConvertMultiDetection", func(t *testing.T) {
			lp := &LoggingPayload{Data: []string{"hello", "world"}}
			t.Run("Multi", func(t *testing.T) {
				lp.IsMulti = true
				msg, err := lp.convert()
				require.NoError(t, err)
				msgs := requireIsGroup(t, 2, msg)

				assert.Equal(t, "hello", msgs[0].String())
				assert.Equal(t, "world", msgs[1].String())
			})
			t.Run("Single", func(t *testing.T) {
				lp.IsMulti = false
				msg, err := lp.convert()
				require.NoError(t, err)
				_, ok := msg.(*message.GroupComposer)
				assert.False(t, ok)
				assert.Equal(t, "hello world", msg.String())
			})
		})
	})
}

type ex struct{}

func (ex) String() string { return "hello world!" }

func requireIsGroup(t *testing.T, size int, msg message.Composer) []message.Composer {
	gc, ok := msg.(*message.GroupComposer)
	require.True(t, ok)
	msgs := gc.Messages()
	require.Len(t, msgs, size)
	return msgs
}
