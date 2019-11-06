package apm

import (
	"testing"

	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvent(t *testing.T) {
	t.Run("Message", func(t *testing.T) {
		t.Run("Empty", func(t *testing.T) {
			e := &eventWindow{}
			fields, ok := e.Message().Raw().(message.Fields)
			require.True(t, ok)
			assert.Len(t, fields, 3)
		})
		t.Run("Populated", func(t *testing.T) {
			e := &eventWindow{
				data: map[eventKey]*eventRecord{
					{dbName: "db", collName: "coll", cmdName: "find"}: &eventRecord{},
				},
			}

			fields, ok := e.Message().Raw().(message.Fields)
			require.True(t, ok)
			assert.Len(t, fields, 4)
		})
	})
	t.Run("Document", func(t *testing.T) {
		t.Run("Empty", func(t *testing.T) {
			e := &eventWindow{}
			doc := e.Document()

			assert.Equal(t, 2, doc.Len())
			assert.Equal(t, 0, doc.Lookup("events").MutableDocument().Len())
		})
		t.Run("Populated", func(t *testing.T) {
			e := &eventWindow{
				data: map[eventKey]*eventRecord{
					{dbName: "db", collName: "coll", cmdName: "find"}: &eventRecord{},
				},
			}
			doc := e.Document()
			assert.Equal(t, 2, doc.Len())
			assert.Equal(t, 1, doc.Lookup("events").MutableDocument().Len())
		})
	})
}
