package model

import "time"

type MockModel struct {
	FieldId   string
	FieldInt1 int
	FieldInt2 int
	FieldMap  map[string]string

	FieldStruct *MockSubStruct
}

type MockSubStruct struct {
	SubInt int
}

func (m *MockModel) ToService() (interface{}, error) {
	return nil, nil
}

func (m *MockModel) BuildFromService(in interface{}) error {
	return nil
}

type MockScalars struct {
	TimeType time.Time
	MapType  map[string]interface{}
	AnyType  interface{}
}

type MockEmbedded struct {
	One MockLayerOne
}

type MockLayerOne struct {
	Two MockLayerTwo
}

type MockLayerTwo struct {
	SomeField string
}
