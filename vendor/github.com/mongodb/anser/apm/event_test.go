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
			assert.Len(t, fields, 4)
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
	t.Run("NumTags", func(t *testing.T) {
		e := &eventWindow{}
		assert.Nil(t, e.tags)
		t.Run("SelectedTags", func(t *testing.T) {
			assert.Equal(t, 0, e.numTags(&eventRecord{}))
			e.tags = []string{"one", "two"}
			assert.Equal(t, 2, e.numTags(&eventRecord{}))
		})
		t.Run("AllTags", func(t *testing.T) {
			e.allTags = true
			assert.Equal(t, 0, e.numTags(&eventRecord{}))
			assert.Equal(t, 1, e.numTags(&eventRecord{Tags: map[string]int64{"one": 47}}))
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
		t.Run("AllTags", func(t *testing.T) {
			e := &eventWindow{
				allTags: true,
				data: map[eventKey]*eventRecord{
					{dbName: "db", collName: "coll", cmdName: "find"}: &eventRecord{Tags: map[string]int64{"one": 80}},
				},
			}
			doc := e.Document().Lookup("events").MutableDocument().Lookup("db.coll.find").MutableDocument()
			assert.Equal(t, 4, doc.Len())
			assert.Equal(t, 80, doc.Lookup("one").Int())
		})
		t.Run("AllTags", func(t *testing.T) {
			e := &eventWindow{
				tags: []string{"one", "two"},
				data: map[eventKey]*eventRecord{
					{dbName: "db", collName: "coll", cmdName: "find"}: &eventRecord{Tags: map[string]int64{"one": 80}},
				},
			}
			doc := e.Document().Lookup("events").MutableDocument().Lookup("db.coll.find").MutableDocument()
			assert.Equal(t, 5, doc.Len())
			assert.Equal(t, 80, doc.Lookup("one").Int())
			assert.Equal(t, 0, doc.Lookup("two").Int())
		})
	})
}
