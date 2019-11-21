package apm

import (
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/grip/message"
)

type eventKey struct {
	dbName   string
	cmdName  string
	collName string
}

func (k eventKey) String() string { return fmt.Sprintf("%s.%s.%s", k.dbName, k.collName, k.cmdName) }
func (k eventKey) isNil() bool    { return k.dbName == "" && k.cmdName == "" && k.collName == "" }

type eventRecord struct {
	Failed    int64         `bson:"failed" json:"failed" yaml:"failed"`
	Succeeded int64         `bson:"succeeded" json:"succeeded" yaml:"succeeded"`
	Duration  time.Duration `bson:"duration" json:"duration" yaml:"duration"`
	mutex     sync.RWMutex
}

type eventWindow struct {
	timestamp time.Time
	data      map[eventKey]*eventRecord
}

func (e *eventWindow) Message() message.Composer {
	out := message.Fields{
		"start_at": e.timestamp,
		"message":  "apm-event",
	}

	type output struct {
		Operation  string  `bson:"operation" json:"operation" yaml:"operation"`
		Duration   float64 `bson:"duration_secs" json:"duration_secs" yaml:"duration_secs"`
		Succeeded  int64   `bson:"succeeded" json:"succeeded" yaml:"succeeded"`
		Failed     int64   `bson:"failed" json:"failed" yaml:"failed"`
		Database   string  `bson:"database" json:"database" yaml:"database"`
		Collection string  `bson:"collection" json:"collection" yaml:"collection"`
		Command    string  `bson:"command" json:"command" yaml:"command"`
	}
	colls := make([]output, 0, len(e.data))
	for k, v := range e.data {
		v.mutex.RLock()
		colls = append(colls, output{
			Operation:  k.String(),
			Database:   k.dbName,
			Collection: k.collName,
			Command:    k.cmdName,
			Duration:   v.Duration.Seconds(),
			Succeeded:  v.Succeeded,
			Failed:     v.Failed,
		})
		v.mutex.RUnlock()
	}
	out["collections"] = colls

	return message.MakeFields(out)
}

func (e *eventWindow) Document() *birch.Document {
	payload := birch.DC.Make(len(e.data))
	for k, v := range e.data {
		v.mutex.RLock()
		payload.Append(birch.EC.SubDocument(k.String(),
			birch.DC.Elements(
				birch.EC.Int64("failed", v.Failed),
				birch.EC.Int64("success", v.Succeeded),
				birch.EC.Duration("duration", v.Duration))))
		v.mutex.RUnlock()
	}

	return birch.DC.Elements(
		birch.EC.Time("ts", e.timestamp),
		birch.EC.SubDocument("events", payload.Sorted()),
	)
}
