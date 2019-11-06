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
	mutex     sync.Mutex
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
	for k, v := range e.data {
		out[k.String()] = v
	}
	return message.MakeFields(out)
}

func (e *eventWindow) Document() *birch.Document {
	payload := birch.DC.Make(len(e.data))
	for k, v := range e.data {
		payload.Append(birch.EC.SubDocument(k.String(),
			birch.DC.Elements(
				birch.EC.Int64("failed", v.Failed),
				birch.EC.Int64("success", v.Succeeded),
				birch.EC.Duration("duration", v.Duration))))

	}

	return birch.DC.Elements(
		birch.EC.Time("ts", e.timestamp),
		birch.EC.SubDocument("events", payload.Sorted()),
	)
}
