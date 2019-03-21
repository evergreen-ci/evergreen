package job

import (
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
)

// RegisterDefaultJobs registers all default job types in the amboy
// Job registry which permits their use in contexts that require
// serializing jobs to or from a common format (e.g. queues that
// persist pending and completed jobs outside of the process,) or the
// REST interface.
//
// In most applications these registrations happen automatically in
// the context of package init() functions, but for the
// default/generic jobs, users must explicitly load them into the
// registry.
func RegisterDefaultJobs() {
	registry.AddJobType("shell", func() amboy.Job {
		return NewShellJobInstance()
	})
	grip.Info("registered 'shell' job type")

	registry.AddJobType("group", func() amboy.Job {
		return newGroupInstance()
	})
	grip.Info("registered 'group' job type")
}
