package registry

import "github.com/mongodb/amboy"

// JobFactory is an alias for a function that returns a Job
// interface. All Job implementation should have a factory function
// with this signature to use with the amboy.RegisterJobType and
// amboy.JobFactory functions that use an internal registry of jobs to
// handle correct serialization and de-serialization of job objects.
type JobFactory func() amboy.Job

// AddJobType adds a job type to the amboy package's internal
// registry of job types. This registry is used to support
// serialization and de-serialization of between persistence layers.
func AddJobType(name string, f JobFactory) {
	amboyRegistry.registerJobType(name, f)
}

// GetJobFactory produces Job objects of specific implementations
// based on the type name, used in RegisterJobType and in the
// JobType.Name field.
func GetJobFactory(name string) (JobFactory, error) {
	return amboyRegistry.getJobFactory(name)
}

// JobTypeNames returns an iterator of all registered Job types
func JobTypeNames() <-chan string {
	return amboyRegistry.jobTypeNames()
}
