package registry

import (
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/pkg/errors"
)

// JobInterchange provides a consistent way to describe and reliably
// serialize Job objects between different queue
// instances. Interchange is also used internally as part of JobGroup
// Job type.
type JobInterchange struct {
	Name       string                 `json:"name" bson:"_id" yaml:"name"`
	Type       string                 `json:"type" bson:"type" yaml:"type"`
	Group      string                 `bson:"group,omitempty" json:"group,omitempty" yaml:"group,omitempty"`
	Version    int                    `json:"version" bson:"version" yaml:"version"`
	Priority   int                    `json:"priority" bson:"priority" yaml:"priority"`
	Status     amboy.JobStatusInfo    `bson:"status" json:"status" yaml:"status"`
	Scopes     []string               `bson:"scopes,omitempty" json:"scopes,omitempty" yaml:"scopes,omitempty"`
	TimeInfo   amboy.JobTimeInfo      `bson:"time_info" json:"time_info,omitempty" yaml:"time_info,omitempty"`
	Job        *rawJob                `json:"job,omitempty" bson:"job,omitempty" yaml:"job,omitempty"`
	Dependency *DependencyInterchange `json:"dependency,omitempty" bson:"dependency,omitempty" yaml:"dependency,omitempty"`
}

// MakeJobInterchange changes a Job interface into a JobInterchange
// structure, for easier serialization.
func MakeJobInterchange(j amboy.Job, f amboy.Format) (*JobInterchange, error) {
	typeInfo := j.Type()

	if typeInfo.Version < 0 {
		return nil, errors.New("cannot use jobs with versions less than 0 with job interchange")
	}

	dep, err := makeDependencyInterchange(f, j.Dependency())
	if err != nil {
		return nil, err
	}

	data, err := convertTo(f, j)
	if err != nil {
		return nil, err
	}

	output := &JobInterchange{
		Name:     j.ID(),
		Type:     typeInfo.Name,
		Version:  typeInfo.Version,
		Priority: j.Priority(),
		Status:   j.Status(),
		TimeInfo: j.TimeInfo(),
		Job: &rawJob{
			Body: data,
			Type: typeInfo.Name,
			job:  j,
		},
		Dependency: dep,
	}

	return output, nil
}

// Resolve reverses the process of ConvertToInterchange and
// converts the interchange format to a Job object using the types in
// the registry. Returns an error if the job type of the
// JobInterchange object isn't registered or the current version of
// the job produced by the registry is *not* the same as the version
// of the Job.
func (j *JobInterchange) Resolve(f amboy.Format) (amboy.Job, error) {
	factory, err := GetJobFactory(j.Type)
	if err != nil {
		return nil, err
	}

	job := factory()

	if job.Type().Version != j.Version {
		return nil, errors.Errorf("job '%s' (version=%d) does not match the current version (%d) for the job type '%s'",
			j.Name, j.Version, job.Type().Version, j.Type)
	}

	dep, err := convertToDependency(f, j.Dependency)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	err = convertFrom(f, j.Job.Body, job)
	if err != nil {
		return nil, errors.Wrap(err, "converting job body")
	}

	job.SetDependency(dep)
	job.SetPriority(j.Priority)
	job.SetStatus(j.Status)
	job.UpdateTimeInfo(j.TimeInfo)
	job.SetScopes(j.Scopes)

	return job, nil
}

// Raw returns the serialized version of the job.
func (j *JobInterchange) Raw() []byte { return j.Job.Body }

////////////////////////////////////////////////////////////////////////////////////////////////////

// DependencyInterchange objects are a standard form for
// dependency.Manager objects. Amboy (should) only pass
// DependencyInterchange objects between processes, which have the
// type information in easy to access and index-able locations.
type DependencyInterchange struct {
	Type       string         `json:"type" bson:"type" yaml:"type"`
	Version    int            `json:"version" bson:"version" yaml:"version"`
	Edges      []string       `bson:"edges" json:"edges" yaml:"edges"`
	Dependency *rawDependency `json:"dependency" bson:"dependency" yaml:"dependency"`
}

// MakeDependencyInterchange converts a dependency.Manager document to
// its DependencyInterchange format.
func makeDependencyInterchange(f amboy.Format, d dependency.Manager) (*DependencyInterchange, error) {
	typeInfo := d.Type()

	data, err := convertTo(f, d)
	if err != nil {
		return nil, err
	}

	output := &DependencyInterchange{
		Type:    typeInfo.Name,
		Version: typeInfo.Version,
		Edges:   d.Edges(),
		Dependency: &rawDependency{
			Body: data,
			Type: typeInfo.Name,
			dep:  d,
		},
	}

	return output, nil
}

// convertToDependency uses the registry to convert a
// DependencyInterchange object to the correct dependnecy.Manager
// type.
func convertToDependency(f amboy.Format, d *DependencyInterchange) (dependency.Manager, error) {
	factory, err := GetDependencyFactory(d.Type)
	if err != nil {
		return nil, err
	}

	dep := factory()

	if dep.Type().Version != d.Version {
		return nil, errors.Errorf("dependency '%s' (version=%d) does not match the current version (%d) for the dependency type '%s'",
			d.Type, d.Version, dep.Type().Version, dep.Type().Name)
	}

	// this works, because we want to use all the data from the
	// interchange object, but want to use the type information
	// associated with the object that we produced with the
	// factory.
	err = convertFrom(f, d.Dependency.Body, dep)
	if err != nil {
		return nil, errors.Wrap(err, "converting dependency")
	}

	return dep, nil
}
