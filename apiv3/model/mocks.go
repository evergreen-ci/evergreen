package model

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

func (m *MockModel) FromService(in interface{}) error {
	return nil
}
