package task

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

type Tasks []Task

func (t Tasks) Len() int           { return len(t) }
func (t Tasks) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t Tasks) Less(i, j int) bool { return t[i].Id < t[j].Id }

func (t Tasks) getPayload() []interface{} {
	payload := make([]interface{}, len(t))
	for idx := range t {
		payload[idx] = interface{}(t[idx])
	}

	if len(t) > 0 && t[0].BuildVariant == "rhel-62-64-bit-mobile" {
		grip.Debug(message.Fields{
			"ticket":  "EVG-5226",
			"version": t[0].Version,
			"payload": t,
		})
	}

	return payload
}

func (t Tasks) Insert() error {
	return db.InsertMany(Collection, t.getPayload()...)
}

func (t Tasks) InsertUnordered() error {
	return db.InsertManyUnordered(Collection, t.getPayload()...)
}

type ByPriority []string

func (p ByPriority) Len() int           { return len(p) }
func (p ByPriority) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p ByPriority) Less(i, j int) bool { return displayTaskPriority(p[i]) < displayTaskPriority(p[j]) }
