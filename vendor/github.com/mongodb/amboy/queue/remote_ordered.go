package queue

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// SimpleRemoteOrdered queue implements the amboy.Queue interface and
// uses a driver backend, like the RemoteUnordered queue. However,
// this implementation evaluates and respects dependencies, unlike the
// RemoteUnordered implementation which executes all tasks.
//
// The term simple differentiates from a queue that schedules tasks in
// order based on reported edges, which may be more efficient with
// more complex dependency graphs. Internally SimpleRemoteOrdered and
// RemoteUnordred share an implementation *except* for the Next method,
// which differs in task dispatching strategies.
type remoteSimpleOrdered struct {
	*remoteBase
}

// newSimpleRemoteOrdered returns a queue with a configured local
// runner with the specified number of workers.
func newSimpleRemoteOrdered(size int) remoteQueue {
	q := &remoteSimpleOrdered{remoteBase: newRemoteBase()}
	q.dispatcher = NewDispatcher(q)
	grip.Error(q.SetRunner(pool.NewLocalWorkers(size, q)))
	grip.Infof("creating new remote job queue with %d workers", size)

	return q
}

// Next contains the unique implementation details of the
// SimpleRemoteOrdered queue. It fetches a job from the backend,
// skipping all jobs that are: locked (in progress elsewhere,) marked
// as "Passed" (all work complete,) and Unresolvable (e.g. stuck). For
// jobs that are Ready to run, it dispatches them immediately.
//
// For job that are Blocked, Next also skips these jobs *but* in hopes
// that the next time this job is dispatched its dependencies will be
// ready. if there is only one Edge reported, blocked will attempt to
// dispatch the dependent job.
func (q *remoteSimpleOrdered) Next(ctx context.Context) amboy.Job {
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil
		case job := <-q.channel:
			ti := amboy.JobTimeInfo{
				Start: time.Now(),
			}
			job.UpdateTimeInfo(ti)

			dep := job.Dependency()

			// The final statement ensures that jobs are dispatched in "order," or at
			// least that jobs are only dispatched when "ready" and that jobs that are
			// blocked, should wait for their dependencies to complete.
			//
			// The local version of this queue reads all jobs in and builds a DAG, which
			// it then sorts and executes in order. This takes a more rudimentary approach.
			id := job.ID()
			switch dep.State() {
			case dependency.Ready:
				grip.Debugf("returning job %s from remote source, duration = %s",
					id, time.Since(start))
				return job
			case dependency.Passed:
				q.dispatcher.Release(ctx, job)
				q.addBlocked(job.ID())
				continue
			case dependency.Unresolved:
				grip.Warning(message.MakeFieldsMessage("detected a dependency error",
					message.Fields{
						"job_id": id,
						"edges":  dep.Edges(),
						"dep":    dep.Type(),
					}))
				q.dispatcher.Release(ctx, job)
				q.addBlocked(job.ID())
				continue
			case dependency.Blocked:
				edges := dep.Edges()
				grip.Debug(message.Fields{
					"job_id":    id,
					"edges":     edges,
					"dep":       dep.Type(),
					"job_type":  job.Type().Name,
					"num_edges": len(edges),
					"message":   "job is blocked",
				})

				if len(edges) == 1 {
					// this is just an optimization; if there's one dependency it's easy
					// to move that job up in the queue by submitting it here.
					dj, ok := q.Get(ctx, edges[0])
					if ok && isDispatchable(dj.Status(), q.Info().LockTimeout) {
						// might need to make this non-blocking.
						q.dispatcher.Release(ctx, job)
						if q.dispatcher.Dispatch(ctx, dj) == nil {
							q.channel <- dj
						}
						continue
					}
				}

				q.dispatcher.Release(ctx, job)
				q.addBlocked(id)

				continue
			default:
				q.dispatcher.Release(ctx, job)
				grip.Warning(message.MakeFieldsMessage("detected invalid dependency",
					message.Fields{
						"job_id": id,
						"edges":  dep.Edges(),
						"dep":    dep.Type(),
						"state": message.Fields{
							"value":  dep.State(),
							"valid":  dependency.IsValidState(dep.State()),
							"string": dep.State().String(),
						},
					}))
			}
		}
	}

}
