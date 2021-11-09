package apm

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/evergreen-ci/birch"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
)

type basicMonitor struct {
	config *MonitorConfig

	inProg     map[int64]eventKey
	inProgLock sync.Mutex

	current        map[eventKey]*eventRecord
	currentStartAt time.Time
	currentLock    sync.Mutex
}

// NewBasicMonitor returns a simple monitor implementation that does
// not automatically rotate data. The MonitorConfig makes it possible to
// filter events. If this value is nil, no events will be filtered.
func NewBasicMonitor(config *MonitorConfig) Monitor {
	if config != nil && len(config.Tags) > 0 {
		sort.Strings(config.Tags)
	}

	return &basicMonitor{
		config:         config,
		inProg:         make(map[int64]eventKey),
		current:        make(map[eventKey]*eventRecord),
		currentStartAt: time.Now(),
	}
}

func (m *basicMonitor) popRequest(id int64) eventKey {
	m.inProgLock.Lock()
	defer m.inProgLock.Unlock()

	out, ok := m.inProg[id]
	if ok {
		delete(m.inProg, id)
	}

	return out
}

func (m *basicMonitor) setRequest(id int64, key eventKey) {
	if !m.config.shouldTrack(key) {
		return
	}

	m.inProgLock.Lock()
	defer m.inProgLock.Unlock()

	m.inProg[id] = key
}

func (m *basicMonitor) getRecord(id int64) *eventRecord {
	key := m.popRequest(id)
	if key.isNil() {
		return nil
	}

	m.currentLock.Lock()
	defer m.currentLock.Unlock()

	e := m.current[key]
	if e == nil {
		e = &eventRecord{
			Tags: map[string]int64{},
		}
		m.current[key] = e
	}

	return e
}

func resolveCollectionName(raw bson.Raw, name string) (collection string) {
	doc, err := birch.DC.ReaderErr(birch.Reader(raw))
	if err != nil {
		return
	}

	switch name {
	case "getMore":
		collection, _ = doc.Lookup("collection").StringValueOK()
	default:
		collection, _ = doc.Lookup(name).StringValueOK()
	}

	return
}

func (m *basicMonitor) DriverAPM() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started: func(ctx context.Context, e *event.CommandStartedEvent) {
			m.setRequest(e.RequestID, eventKey{
				dbName:   e.DatabaseName,
				cmdName:  e.CommandName,
				collName: resolveCollectionName(e.Command, e.CommandName),
			})
		},
		Succeeded: func(ctx context.Context, e *event.CommandSucceededEvent) {
			event := m.getRecord(e.RequestID)
			if event == nil {
				return
			}

			event.mutex.Lock()
			defer event.mutex.Unlock()

			event.Succeeded++
			event.Duration += time.Duration(e.DurationNanos)

			m.addTags(ctx, event)
		},
		Failed: func(ctx context.Context, e *event.CommandFailedEvent) {
			event := m.getRecord(e.RequestID)
			if event == nil {
				return
			}

			event.mutex.Lock()
			defer event.mutex.Unlock()

			event.Failed++
			event.Duration += time.Duration(e.DurationNanos)

			m.addTags(ctx, event)
		},
	}
}

func (m *basicMonitor) addTags(ctx context.Context, event *eventRecord) {
	if m.config == nil {
		return
	}

	for _, tag := range GetTags(ctx) {
		if m.config.AllTags || stringSliceContains(m.config.Tags, tag) {
			event.Tags[tag]++
		}
	}
}

func (m *basicMonitor) Rotate() Event {
	newWindow := m.config.window()

	m.currentLock.Lock()
	defer m.currentLock.Unlock()

	out := &eventWindow{
		data:      m.current,
		timestamp: m.currentStartAt,
	}

	if m.config != nil {
		out.allTags = m.config.AllTags
		out.tags = m.config.Tags
	}

	m.current = newWindow
	m.currentStartAt = time.Now()
	return out
}
