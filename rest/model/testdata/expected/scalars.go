// Code generated by rest/model/codegen.go. DO NOT EDIT.

package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/rest/model"
)

type APIMockScalars struct {
	TimeType *time.Time             `json:"time_type"`
	MapType  map[string]interface{} `json:"map_type"`
	AnyType  interface{}            `json:"any_type"`
}

func APIMockScalarsBuildFromService(t model.MockScalars) *APIMockScalars {
	m := APIMockScalars{}
	m.AnyType = InterfaceInterface(t.AnyType)
	m.MapType = MapstringinterfaceMapstringinterface(t.MapType)
	m.TimeType = TimeTimeTimeTimePtr(t.TimeType)
	return &m
}

func APIMockScalarsToService(m *APIMockScalars) model.MockScalars {
	out := model.MockScalars{}
	out.AnyType = InterfaceInterface(m.AnyType)
	out.MapType = MapstringinterfaceMapstringinterface(m.MapType)
	out.TimeType = TimeTimeTimeTimePtr(m.TimeType)
	return out
}
