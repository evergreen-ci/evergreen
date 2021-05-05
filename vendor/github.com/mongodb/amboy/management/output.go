package management

// JobTypeCount holds data for counts of jobs by job type and group.
type JobTypeCount struct {
	Type  string
	Group string
	Count int
}

// GroupedID represents a job's ID and the group that the job belongs to, if
// it's in a queue group.
type GroupedID struct {
	ID    string `bson:"_id" bson:"_id" yaml:"_id"`
	Group string `bson:"group,omitempty" json:"group,omitempty" yaml:"group,omitempty"`
}
