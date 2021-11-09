package registry

import mgobson "gopkg.in/mgo.v2/bson"

type rawJob []byte

func (j rawJob) MarshalJSON() ([]byte, error)   { return j, nil }
func (j *rawJob) UnmarshalBSON(in []byte) error { *j = in; return nil }
func (j rawJob) MarshalBSON() ([]byte, error)   { return j, nil }
func (j *rawJob) UnmarshalJSON(in []byte) error { *j = in; return nil }
func (j *rawJob) SetBSON(r mgobson.Raw) error   { *j = r.Data; return nil }
func (j *rawJob) GetBSON() (interface{}, error) { return *j, nil }

type rawDependency []byte

func (d rawDependency) MarshalJSON() ([]byte, error)   { return d, nil }
func (d *rawDependency) UnmarshalJSON(in []byte) error { *d = in; return nil }
func (d rawDependency) MarshalBSON() ([]byte, error)   { return d, nil }
func (d *rawDependency) UnmarshalBSON(in []byte) error { *d = in; return nil }
func (d *rawDependency) SetBSON(r mgobson.Raw) error   { *d = r.Data; return nil }
func (d *rawDependency) GetBSON() (interface{}, error) { return d, nil }
