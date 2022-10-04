package task

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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

func (t Tasks) Export() []Task {
	out := make([]Task, len(t))
	for idx := range t {
		out[idx] = *t[idx]
	}
	return out
}

func (t Tasks) Insert() error {
	return db.InsertMany(Collection, t.getPayload()...)
}

func (t Tasks) InsertUnordered(ctx context.Context) error {
	if t.Len() == 0 {
		return nil
	}
	ordered := false
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertMany(ctx, t.getPayload(), &options.InsertManyOptions{Ordered: &ordered})
	return err
}

// ByPriority sorts tasks according to their display statuses (and has nothing
// to do with its scheduling priority).
type ByPriority []Task

func (p ByPriority) Len() int      { return len(p) }
func (p ByPriority) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p ByPriority) Less(i, j int) bool {
	return p[i].displayTaskPriority() < p[j].displayTaskPriority()
}
