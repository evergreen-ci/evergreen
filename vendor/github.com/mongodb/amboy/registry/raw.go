package registry

import (
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
	legacyBSON "gopkg.in/mgo.v2/bson"
)

type rawJob struct {
	Body []byte `bson:"body" json:"body" yaml:"body"`
	Type string `bson:"type" json:"type" yaml:"type"`
	job  interface{}
}

func (j *rawJob) SetBSON(r legacyBSON.Raw) error { j.Body = r.Data; return nil }
func (j *rawJob) GetBSON() (interface{}, error) { // Get ~= Marshal
	if j.job != nil {
		return j.job, nil
	}

	factory, err := GetJobFactory(j.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	job := factory()
	if err = convertFrom(amboy.BSON, j.Body, job); err != nil {
		return nil, errors.WithStack(err)
	}
	j.job = job

	return j.job, nil
}

func (j *rawJob) UnmarshalYAML(um func(interface{}) error) error {
	factory, err := GetJobFactory(j.Type)
	if err != nil {
		return errors.WithStack(err)
	}

	job := factory()

	err = um(job)
	if err != nil {
		return err
	}

	j.job = job

	j.Body, err = convertTo(amboy.YAML, job)
	return err
}
func (j *rawJob) MarshalYAML() (interface{}, error) {
	if j.job != nil {
		return j.job, nil
	}

	factory, err := GetJobFactory(j.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	job := factory()
	if err = convertFrom(amboy.YAML, j.Body, job); err != nil {
		return nil, errors.WithStack(err)
	}
	j.job = job

	return j.job, nil
}

func (j *rawJob) UnmarshalJSON(in []byte) error { j.Body = in; return nil }
func (j *rawJob) MarshalJSON() ([]byte, error) {
	if j.Body != nil {
		return j.Body, nil
	}

	if j.job == nil {
		return nil, errors.New("nil job defined")
	}

	var err error

	j.Body, err = convertTo(amboy.JSON, j.job)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return j.Body, nil
}

func (j *rawJob) UnmarshalBSON(in []byte) error { j.Body = in; return nil }
func (j *rawJob) MarshalBSON() ([]byte, error) {
	if j.Body != nil {
		return j.Body, nil
	}

	if j.job == nil {
		return nil, errors.New("nil job defined")
	}

	var err error

	j.Body, err = convertTo(amboy.BSON2, j.job)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return j.Body, nil
}

////////////////////////////////////////////////////////////////////////

type rawDependency struct {
	Body []byte `bson:"body" json:"body" yaml:"body"`
	Type string `bson:"type" json:"type" yaml:"type"`
	dep  interface{}
}

func (d *rawDependency) SetBSON(r legacyBSON.Raw) error { d.Body = r.Data; return nil }
func (d *rawDependency) GetBSON() (interface{}, error) { // Get ~= Marshal
	if d.dep != nil {
		return d.dep, nil
	}

	factory, err := GetDependencyFactory(d.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dep := factory()

	if err = convertFrom(amboy.BSON, d.Body, dep); err != nil {
		return nil, errors.WithStack(err)
	}

	d.dep = dep

	return d.dep, nil
}

func (d *rawDependency) UnmarshalYAML(um func(interface{}) error) error {
	factory, err := GetDependencyFactory(d.Type)
	if err != nil {
		return errors.WithStack(err)
	}

	dep := factory()
	if err = um(dep); err != nil {
		return errors.WithStack(err)
	}

	d.dep = dep

	d.Body, err = convertTo(amboy.YAML, dep)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (d *rawDependency) MarshalYAML() (interface{}, error) {
	if d.dep != nil {
		return d.dep, nil
	}

	factory, err := GetDependencyFactory(d.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dep := factory()
	if err = convertFrom(amboy.YAML, d.Body, dep); err != nil {
		return nil, errors.WithStack(err)
	}
	d.dep = dep

	return d.dep, nil
}

func (d *rawDependency) UnmarshalJSON(in []byte) error { d.Body = in; return nil }
func (d *rawDependency) MarshalJSON() ([]byte, error) {
	if d.Body != nil {
		return d.Body, nil
	}

	if d.dep == nil {
		return nil, errors.New("nil dependency defined")
	}

	var err error
	d.Body, err = convertTo(amboy.JSON, d.dep)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return d.Body, nil
}

func (d *rawDependency) UnmarshalBSON(in []byte) error { d.Body = in; return nil }
func (d *rawDependency) MarshalBSON() ([]byte, error) {
	if d.Body != nil {
		return d.Body, nil
	}

	if d.dep == nil {
		return nil, errors.New("nil dependency defined")
	}

	var err error
	d.Body, err = convertTo(amboy.BSON2, d.dep)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return d.Body, nil
}
