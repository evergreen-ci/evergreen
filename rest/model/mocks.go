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

func (m *MockModel) ToService() (any, error) {
	return nil, nil
}

func (m *MockModel) BuildFromService(in any) error {
	return nil
}

type MockScalars struct {
	TimeType time.Time
	MapType  map[string]any
	AnyType  any
}

type MockEmbedded struct {
	One MockLayerOne
}

type MockLayerOne struct {
	Two MockLayerTwo
}

type MockLayerTwo struct {
	SomeField *string
}

type MockTypes struct {
	BoolType       bool
	BoolPtrType    *bool
	IntType        int
	IntPtrType     *int
	StringType     string
	StringPtrType  *string
	Uint64Type     uint64
	Uint64PtrType  *uint64
	Float64Type    float64
	Float64PtrType *float64
	RuneType       rune
	RunePtrType    *rune
}

type StructWithAliased struct {
	Foo AliasedType
	Bar string
}

type AliasedType string
