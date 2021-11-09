package gimlet

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type SampleSubStruct struct {
	Sub1  string `csv:"this"`
	NoCSV string
}

type SampleStruct struct {
	FieldA    string          `csv:"fieldA"`
	FieldInt  int             `csv:"fieldB"`
	FieldBool bool            `csv:"boolean"`
	Sub1      SampleSubStruct `csv:"sub1"`
	Sub2      SampleSubStruct
	NoCSV     string
}

func sampleGenerator() []SampleStruct {
	return []SampleStruct{
		{
			FieldA:    "hello",
			FieldInt:  1,
			FieldBool: true,
			Sub1:      SampleSubStruct{"this", "print"},
			Sub2:      SampleSubStruct{"another", "no"},
			NoCSV:     "hi",
		},
		{
			FieldA:    "world",
			FieldInt:  1,
			FieldBool: true,
			Sub1:      SampleSubStruct{"mongo", "db"},
			Sub2:      SampleSubStruct{"ever", "green"},
			NoCSV:     "hi",
		},
	}
}

func TestCSV(t *testing.T) {
	t.Run("ResolveFieldsAndValues", func(t *testing.T) {
		t.Run("Fields", func(t *testing.T) {
			assert.Equal(t, []string{"fieldA", "fieldB", "boolean", "this", "this"}, getCSVFields(reflect.TypeOf(SampleStruct{})))
		})
		t.Run("Values", func(t *testing.T) {
			s := SampleStruct{
				FieldA:    "hello",
				FieldInt:  1,
				FieldBool: true,
				Sub1:      SampleSubStruct{"this", "print"},
				Sub2:      SampleSubStruct{"another", "no"},
				NoCSV:     "hi",
			}
			values := getCSVValues(s)
			assert.Equal(t, []string{"hello", "1", "true", "this", "another"}, values)
		})
	})
	t.Run("ConvertValues", func(t *testing.T) {
		t.Run("ErrorSingle", func(t *testing.T) {
			single := sampleGenerator()[0]
			_, err := convertDataToCSVRecord(single)
			assert.Error(t, err)
		})
		t.Run("ErrorEmpty", func(t *testing.T) {
			_, err := convertDataToCSVRecord([]SampleStruct{})
			assert.Error(t, err)
		})
		t.Run("Validation", func(t *testing.T) {
			samples := sampleGenerator()
			strings, err := convertDataToCSVRecord(samples)
			require.NoError(t, err)
			require.Len(t, strings, 3)

			assert.Equal(t, []string{"fieldA", "fieldB", "boolean", "this", "this"}, strings[0])
			assert.Equal(t, []string{"hello", "1", "true", "this", "another"}, strings[1])
			assert.Equal(t, []string{"world", "1", "true", "mongo", "ever"}, strings[2])
		})
	})
	t.Run("ResponseWrappers", func(t *testing.T) {
		t.Run("StatusOK", func(t *testing.T) {
			rsp := httptest.NewRecorder()
			WriteCSV(rsp, sampleGenerator())
			assert.Equal(t, 200, rsp.Code)
			assert.NotNil(t, rsp.Body)
		})
		t.Run("StatusBadRequest", func(t *testing.T) {
			rsp := httptest.NewRecorder()
			WriteCSVError(rsp, sampleGenerator())
			assert.Equal(t, 400, rsp.Code)
			assert.NotNil(t, rsp.Body)
		})
		t.Run("InternalError", func(t *testing.T) {
			rsp := httptest.NewRecorder()
			WriteCSVInternalError(rsp, sampleGenerator())
			assert.Equal(t, 500, rsp.Code)
			assert.NotNil(t, rsp.Body)
		})
		t.Run("BadInputEmpty", func(t *testing.T) {
			rsp := httptest.NewRecorder()
			WriteCSV(rsp, []SampleStruct{})
			assert.Equal(t, 500, rsp.Code)
			assert.NotNil(t, rsp.Body)
		})
		t.Run("BadInputNil", func(t *testing.T) {
			rsp := httptest.NewRecorder()
			WriteCSV(rsp, "hi")
			assert.Equal(t, 500, rsp.Code)
			assert.NotNil(t, rsp.Body)
		})
	})
}
