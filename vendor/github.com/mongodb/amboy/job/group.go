package job

import (
	"context"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Group is a structure for running collections of Job objects at the
// same time, as a single Job. Use Groups to isolate several Jobs from
// other Jobs in the queue, and ensure that several Jobs run on a
// single system.
type Group struct {
	Jobs  map[string]*registry.JobInterchange `bson:"jobs" json:"jobs" yaml:"jobs"`
	*Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	mutex sync.RWMutex
}

// NewGroup creates a new, empty Group object.
func NewGroup(name string) *Group {
	g := newGroupInstance()
	g.SetID(name)

	return g
}

// newGroupInstance is a common constructor for the public NewGroup
// constructior and the registry.JobFactory constructor.
func newGroupInstance() *Group {
	g := &Group{
		Jobs: make(map[string]*registry.JobInterchange),
		Base: &Base{
			JobType: amboy.JobType{
				Name:    "group",
				Version: 1,
			},
		},
	}

	g.Base.SetDependency(dependency.NewAlways())
	return g
}

// Add is not part of the Job interface, but allows callers to append
// jobs to the Group. Returns an error if a job with the same ID()
// value already exists in the group.
func (g *Group) Add(j amboy.Job) error {
	name := j.ID()

	g.mutex.Lock()
	defer g.mutex.Unlock()
	_, exists := g.Jobs[name]
	if exists {
		return errors.Errorf("job named '%s', already exists in Group %s",
			name, g.ID())
	}

	job, err := registry.MakeJobInterchange(j, amboy.JSON)
	if err != nil {
		return err
	}

	g.Jobs[name] = job
	return nil
}

// Run executes the jobs. Provides "continue on error" semantics for
// Jobs in the Group. Returns an error if: the Group has already
// run, or if any of the constituent Jobs produce an error *or* if
// there are problems with the JobInterchange converters.
func (g *Group) Run(ctx context.Context) {
	defer g.MarkComplete()

	if g.Status().Completed {
		g.AddError(errors.Errorf("Group '%s' has already executed", g.ID()))
		return
	}

	wg := &sync.WaitGroup{}

	g.mutex.RLock()
	for _, job := range g.Jobs {
		if err := ctx.Err(); err != nil {
			g.AddError(err)
			break
		}

		runnableJob, err := job.Resolve(amboy.JSON)
		if err != nil {
			g.AddError(err)
			continue
		}

		depState := runnableJob.Dependency().State()
		if depState == dependency.Passed {
			grip.Infof("skipping job %s because of dependency", runnableJob.ID())
			continue
		} else if depState == dependency.Blocked || depState == dependency.Unresolved {
			grip.Warningf("dispatching blocked/unresolved job %s", runnableJob.ID())
		}

		wg.Add(1)
		go func(j amboy.Job, group *Group) {
			defer wg.Done()

			maxTime := j.TimeInfo().MaxTime
			if maxTime > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, maxTime)
				defer cancel()
			}

			j.Run(ctx)

			// after the task completes, add the issue
			// back to Jobs map so that we preserve errors
			// idiomatically for Groups.
			jobErr := j.Error()
			g.AddError(jobErr)

			job, err := registry.MakeJobInterchange(j, amboy.JSON)
			if err != nil {
				g.AddError(err)
				return
			}

			if jobErr != nil {
				return
			}

			group.mutex.Lock()
			defer group.mutex.Unlock()
			group.Jobs[j.ID()] = job
		}(runnableJob, g)
	}
	g.mutex.RUnlock()
	wg.Wait()

	g.MarkComplete()
}

// SetDependency allows you to configure the dependency.Manager
// instance for this object. If you want to swap different dependency
// instances you can as long as the new instance is of the "Always"
// type.
func (g *Group) SetDependency(d dependency.Manager) {
	if d == nil || d.Type().Name != "always" {
		grip.Warningf("group job types must have 'always' dependency types, '%s' is invalid",
			d.Type().Name)
		return
	}

	g.Base.SetDependency(d)
}
