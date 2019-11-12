package apm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringSliceContains(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		assert.False(t, stringSliceContains(nil, "foo"))
		assert.False(t, stringSliceContains([]string{}, "foo"))
	})
	t.Run("Exists", func(t *testing.T) {
		assert.True(t, stringSliceContains([]string{"foo"}, "foo"))
		assert.True(t, stringSliceContains([]string{"", "foo"}, "foo"))
	})
	t.Run("DoesNotExist", func(t *testing.T) {
		assert.False(t, stringSliceContains([]string{"foo"}, "bar"))
		assert.False(t, stringSliceContains([]string{"", "foo"}, "bar"))
	})
}

func TestMonitorConfig(t *testing.T) {
	t.Run("Tracking", func(t *testing.T) {
		t.Run("Nil", func(t *testing.T) {
			var conf *MonitorConfig
			assert.Nil(t, conf)
			require.NotPanics(t, func() {
				assert.True(t, conf.shouldTrack(eventKey{}))
			})
		})
		t.Run("Empty", func(t *testing.T) {
			conf := &MonitorConfig{}
			assert.True(t, conf.shouldTrack(eventKey{}))
		})
		t.Run("PopulatedEmpty", func(t *testing.T) {
			conf := &MonitorConfig{
				Commands:    []string{""},
				Databases:   []string{""},
				Collections: []string{""},
			}
			assert.True(t, conf.shouldTrack(eventKey{}))
		})
		t.Run("PopulatedSingle", func(t *testing.T) {
			conf := &MonitorConfig{
				Commands:    []string{"cmd"},
				Databases:   []string{"db"},
				Collections: []string{"coll"},
			}
			assert.False(t, conf.shouldTrack(eventKey{}))
		})
		t.Run("PopulatedPartial", func(t *testing.T) {
			conf := &MonitorConfig{
				Commands:    []string{"cmd"},
				Databases:   []string{"db"},
				Collections: []string{"coll"},
			}
			assert.False(t, conf.shouldTrack(eventKey{dbName: "db"}))
			assert.False(t, conf.shouldTrack(eventKey{dbName: "db", collName: "coll"}))
			assert.True(t, conf.shouldTrack(eventKey{dbName: "db", collName: "coll", cmdName: "cmd"}))
		})
		t.Run("PartialNS", func(t *testing.T) {
			conf := &MonitorConfig{
				Databases:   []string{"db"},
				Collections: []string{"coll"},
			}
			assert.False(t, conf.shouldTrack(eventKey{dbName: "db"}))
			assert.True(t, conf.shouldTrack(eventKey{dbName: "db", collName: "coll"}))
		})
		t.Run("PartialDB", func(t *testing.T) {
			conf := &MonitorConfig{
				Databases: []string{"db"},
				Commands:  []string{"cmd"},
			}
			assert.False(t, conf.shouldTrack(eventKey{dbName: "db"}))
			assert.True(t, conf.shouldTrack(eventKey{dbName: "db", collName: "coll", cmdName: "cmd"}))
		})
		t.Run("Namespace", func(t *testing.T) {
			conf := &MonitorConfig{
				Commands: []string{"find", "insert", "remove", "aggregate"},
				Namespaces: []Namespace{
					{DB: "evergreen", Collection: "host"},
					{DB: "evergreen", Collection: "task"},
					{DB: "amboy", Collection: "service.jobs"},
					{DB: "amboy", Collection: "service.group"},
				},
			}
			assert.False(t, conf.shouldTrack(eventKey{dbName: "evergreen", collName: "host", cmdName: "getMore"}))
			assert.True(t, conf.shouldTrack(eventKey{dbName: "evergreen", collName: "host", cmdName: "find"}))
			assert.False(t, conf.shouldTrack(eventKey{dbName: "amboy", collName: "host", cmdName: "find"}))
		})
	})
	t.Run("WindowConstruction", func(t *testing.T) {
		t.Run("Nil", func(t *testing.T) {
			var conf *MonitorConfig
			assert.Nil(t, conf)
			require.NotPanics(t, func() {
				assert.NotNil(t, conf.window())
				assert.Len(t, conf.window(), 0)
			})
		})
		t.Run("NoEventPopulation", func(t *testing.T) {
			conf := &MonitorConfig{
				Commands:    []string{"cmd"},
				Databases:   []string{"db"},
				Collections: []string{"coll"},
			}
			assert.Len(t, conf.window(), 0)
		})
		t.Run("NoPopulatedEvents", func(t *testing.T) {
			conf := &MonitorConfig{
				PopulateEvents: true,
			}
			assert.Len(t, conf.window(), 0)
		})
		t.Run("PopulatedEventSingle", func(t *testing.T) {
			conf := &MonitorConfig{
				PopulateEvents: true,
				Commands:       []string{"cmd"},
				Databases:      []string{"db"},
				Collections:    []string{"coll"},
			}
			assert.Len(t, conf.window(), 1)
		})
		t.Run("PopulatedEventMulti", func(t *testing.T) {
			conf := &MonitorConfig{
				PopulateEvents: true,
				Commands:       []string{"find", "insert", "remove"},
				Databases:      []string{"anser", "amboy", "evergreen"},
				Collections:    []string{"host", "tasks", "logs"},
			}
			assert.Len(t, conf.window(), 27)
		})
		t.Run("PopulateNamespace", func(t *testing.T) {
			conf := &MonitorConfig{
				PopulateEvents: true,
				Commands:       []string{"find", "insert", "remove", "aggregate"},
				Namespaces: []Namespace{
					{DB: "evergreen", Collection: "host"},
					{DB: "evergreen", Collection: "task"},
					{DB: "amboy", Collection: "service.jobs"},
					{DB: "amboy", Collection: "service.group"},
				},
			}
			assert.Len(t, conf.window(), 16)
		})
	})
}
