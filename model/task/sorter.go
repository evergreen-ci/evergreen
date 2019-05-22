package task

import "github.com/evergreen-ci/evergreen/db"

type Tasks []*Task

func (t Tasks) Len() int           { return len(t) }
func (t Tasks) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t Tasks) Less(i, j int) bool { return t[i].Id < t[j].Id }

func (t Tasks) getPayload() []interface{} {
	payload := make([]interface{}, len(t))
	for idx := range t {
		payload[idx] = interface{}(t[idx])
	}

	return payload
}

func (t Tasks) Insert() error {
	return db.InsertMany(Collection, t.getPayload()...)
}

func (t Tasks) InsertUnordered() error {
	return db.InsertManyUnordered(Collection, t.getPayload()...)
}

type ByPriority []Task

func (p ByPriority) Len() int           { return len(p) }
func (p ByPriority) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p ByPriority) Less(i, j int) bool { return displayTaskPriority(p[i]) < displayTaskPriority(p[j]) }
