// Package units contains amboy.Job definitions.
//
// Loading the units package registers all jobs in the amboy Job
// Registry.
//
// By convention the implementations of these jobs are: private with
// public constructors that return the amboy.Job type, they use
// amboy/job.Base for core implementation, implementing only Run and
// whatever additional methods or functions are required. The units
// package prefers a one-to-one mapping of files to job
// implementations. Additionally all jobs must be capable of
// marshalling to JSON/BSON (depending on the amoby.JobType format.)
package units
