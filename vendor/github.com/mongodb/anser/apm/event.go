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
	Failed    int64            `bson:"failed" json:"failed" yaml:"failed"`
	Succeeded int64            `bson:"succeeded" json:"succeeded" yaml:"succeeded"`
	Duration  time.Duration    `bson:"duration" json:"duration" yaml:"duration"`
	Tags      map[string]int64 `bson:"tags" json:"tags" yaml:"tags"`
	mutex     sync.RWMutex
}

type eventWindow struct {
	timestamp time.Time
	data      map[eventKey]*eventRecord
	tags      []string
	allTags   bool
}

func (e *eventWindow) Message() message.Composer {
	out := message.Fields{
		"start_at": e.timestamp,
		"message":  "apm-event",
	}

	type output struct {
		Operation  string           `bson:"operation" json:"operation" yaml:"operation"`
		Duration   float64          `bson:"duration_secs" json:"duration_secs" yaml:"duration_secs"`
		Succeeded  int64            `bson:"succeeded" json:"succeeded" yaml:"succeeded"`
		Failed     int64            `bson:"failed" json:"failed" yaml:"failed"`
		Database   string           `bson:"database" json:"database" yaml:"database"`
		Collection string           `bson:"collection" json:"collection" yaml:"collection"`
		Command    string           `bson:"command" json:"command" yaml:"command"`
		Tags       map[string]int64 `bson:"tags" json:"tags" yaml:"tags"`
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
			Tags:       v.Tags,
		})
		v.mutex.RUnlock()
	}
	out["collections"] = colls

	return message.MakeFields(out)
}

func (e *eventWindow) numTags(v *eventRecord) int {
	if e.allTags {
		return len(v.Tags)
	}
	return len(e.tags)
}

func (e *eventWindow) Document() *birch.Document {
	payload := birch.DC.Make(len(e.data))

	for k, v := range e.data {
		v.mutex.RLock()
		doc := birch.DC.Make(3+e.numTags(v)).
			Append(birch.EC.Int64("failed", v.Failed),
				birch.EC.Int64("success", v.Succeeded),
				birch.EC.Duration("duration", v.Duration))

		if e.allTags {
			for name, count := range v.Tags {
				doc.Append(birch.EC.Int64(name, count))
			}
		} else {
			for _, t := range e.tags {
				doc.Append(birch.EC.Int64(t, v.Tags[t]))
			}
		}
		v.mutex.RUnlock()
		payload.Append(birch.EC.SubDocument(k.String(), doc.Sorted()))
	}

	return birch.DC.Elements(
		birch.EC.Time("ts", e.timestamp),
		birch.EC.SubDocument("events", payload.Sorted()),
	)
}
