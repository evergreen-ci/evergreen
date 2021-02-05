package model

import (
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type HostListResults struct {
	Hosts   []CreateHost
	Details []APIHostCreateDetail
}

type APIHostCreateDetail struct {
	HostId *string `bson:"host_id" json:"host_id"`
	Error  *string `bson:"error" json:"error"`
}

func (a *APIHostCreateDetail) BuildFromService(t interface{}) error {
	switch v := t.(type) {
	case task.HostCreateDetail:
		a.HostId = utility.ToStringPtr(v.HostId)
		a.Error = utility.ToStringPtr(v.Error)
	default:
		return errors.New("Incorrect type when unmarshalling HostCreateDetail")
	}
	return nil
}

func (a *APIHostCreateDetail) ToService() (interface{}, error) {
	return nil, errors.New("ToService() is not implemented for APIHostCreateDetail")
}
