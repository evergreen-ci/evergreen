package registry

import (
	"encoding/json"

	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type rawJob struct {
	Body json.RawMessage `bson:"body" json:"body" yaml:"body"`
	Type string          `bson:"type" json:"type" yaml:"type"`
	job  interface{}
}

func (j *rawJob) SetBSON(r bson.Raw) error { j.Body = r.Data; return nil }
func (j *rawJob) GetBSON() (interface{}, error) { // Get ~= Marshal
	if j.job != nil {
		return j.job, nil
	}

	factory, err := GetJobFactory(j.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	job := factory()
	if err = amboy.ConvertFrom(amboy.BSON, j.Body, job); err != nil {
		return nil, errors.WithStack(err)
	}
	j.job = job

	return j.job, nil
}

func (j *rawJob) UnmasrhalJSON(data []byte) error { j.Body = data; return nil }

func (j *rawJob) MarshalYAML() (interface{}, error) {
	if j.job != nil {
		return j.job, nil
	}

	factory, err := GetJobFactory(j.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	job := factory()
	if err = amboy.ConvertFrom(amboy.YAML, j.Body, job); err != nil {
		return nil, errors.WithStack(err)
	}
	j.job = job

	return j.job, nil
}

func (j *rawJob) UnmasrhalYAML(unmarshaler func(interface{}) error) error {

	return unmarshaler(j.job)

}

////////////////////////////////////////////////////////////////////////

type rawDependency struct {
	Body []byte `bson:"body" json:"body" yaml:"body"`
	Type string `bson:"type" json:"type" yaml:"type"`
	dep  interface{}
}

func (d *rawDependency) SetBSON(r bson.Raw) error { d.Body = r.Data; return nil }
func (d *rawDependency) GetBSON() (interface{}, error) { // Get ~= Marshal
	if d.dep != nil {
		return d.dep, nil
	}

	factory, err := GetDependencyFactory(d.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dep := factory()

	if err = amboy.ConvertFrom(amboy.BSON, d.Body, dep); err != nil {
		return nil, errors.WithStack(err)
	}

	d.dep = dep

	return d.dep, nil
}

func (d *rawDependency) UnmasrhalJSON(data []byte) error { d.Body = data; return nil }

func (d *rawDependency) MarshalYAML() (interface{}, error) {
	if d.dep != nil {
		return d.dep, nil
	}

	factory, err := GetDependencyFactory(d.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dep := factory()
	if err = amboy.ConvertFrom(amboy.YAML, d.Body, dep); err != nil {
		return nil, errors.WithStack(err)
	}
	d.dep = dep

	return d.dep, nil
}
