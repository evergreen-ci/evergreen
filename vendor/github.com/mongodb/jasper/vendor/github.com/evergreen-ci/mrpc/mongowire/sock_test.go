package mongowire

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

func TestReadMessage(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	smallMessage := createSmallMessage(t)
	smallMessageBytes := smallMessage.Serialize()
	largeMessage := createLargeMessage(t, 3*1024*1024)
	largeMessageBytes := largeMessage.Serialize()

	for _, test := range []struct {
		name            string
		ctx             context.Context
		reader          io.Reader
		expectedMessage Message
		hasErr          bool
	}{
		{
			name:   "EmptyReader",
			ctx:    context.TODO(),
			reader: bytes.NewReader([]byte{}),
			hasErr: true,
		},
		{
			name:   "NoHeader",
			ctx:    context.TODO(),
			reader: bytes.NewReader([]byte{'a', 'b', 'c'}),
			hasErr: true,
		},
		{
			name:   "CanceledContext",
			ctx:    canceled,
			reader: bytes.NewReader(smallMessageBytes),
			hasErr: true,
		},
		{
			name:   "MessageTooLarge",
			ctx:    context.TODO(),
			reader: bytes.NewReader(createLargeMessage(t, 200*1024*1024).Serialize()),
			hasErr: true,
		},
		{
			name:   "InvalidHeaderSize",
			ctx:    context.TODO(),
			reader: bytes.NewReader(int32ToBytes(-1)),
			hasErr: true,
		},
		{
			name:   "InvalidMessageHeader",
			ctx:    context.TODO(),
			reader: bytes.NewReader(append(int32ToBytes(20), bytes.Repeat([]byte{'a'}, 4)...)),
			hasErr: true,
		},
		{
			name:            "SmallMessage",
			ctx:             context.TODO(),
			reader:          bytes.NewReader(smallMessageBytes),
			expectedMessage: smallMessage,
		},
		{
			name:            "LargeMesage",
			ctx:             context.TODO(),
			reader:          bytes.NewReader(largeMessageBytes),
			expectedMessage: largeMessage,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			message, err := ReadMessage(test.ctx, test.reader)
			if test.hasErr {
				assert.Error(t, err)
				assert.Nil(t, message)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedMessage.Header(), message.Header())
				assert.Equal(t, test.expectedMessage.Serialize(), message.Serialize())
			}
		})
	}
}

func TestSendMessage(t *testing.T) {
	t.Run("CanceledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w := &mockWriter{}
		assert.Error(t, SendMessage(ctx, createSmallMessage(t), w))
		assert.Empty(t, w.data)
	})
	t.Run("SmallMessage", func(t *testing.T) {
		w := &mockWriter{}
		smallMessage := createSmallMessage(t)
		require.NoError(t, SendMessage(context.TODO(), smallMessage, w))
		assert.Equal(t, w.data, smallMessage.Serialize())
	})
	t.Run("LargeMessage", func(t *testing.T) {
		w := &mockWriter{}
		largeMessage := createLargeMessage(t, 3*1024*1024)
		require.NoError(t, SendMessage(context.TODO(), largeMessage, w))
		assert.Equal(t, w.data, largeMessage.Serialize())
	})
}

type mockWriter struct {
	data []byte
}

func (w *mockWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func createSmallMessage(t *testing.T) Message {
	data, err := bson.Marshal(bson.M{"foo": "bar"})
	require.NoError(t, err)
	query, err := birch.ReadDocument(data)
	require.NoError(t, err)
	data, err = bson.Marshal(bson.M{"bar": "foo"})
	require.NoError(t, err)
	project, err := birch.ReadDocument(data)
	require.NoError(t, err)

	return NewQuery("ns", 0, 0, 1, query, project)
}

func createLargeMessage(t *testing.T, size int) Message {
	data, err := bson.Marshal(bson.M{"foo": bytes.Repeat([]byte{'a'}, size)})
	doc, err := birch.ReadDocument(data)
	require.NoError(t, err)

	return NewQuery("ns", 0, 0, 1, doc, nil)
}

func int32ToBytes(i int32) []byte {
	data := make([]byte, 4)
	writeInt32(i, data, 0)
	return data
}
