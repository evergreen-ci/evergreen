package util

import (
	"reflect"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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

func TestGetCSVFieldsAndValues(t *testing.T) {
	Convey("With a struct with csv struct tags", t, func() {
		Convey("getting csv fields should return the right fields flattened out", func() {
			fields := getCSVFields(reflect.TypeOf(SampleStruct{}))
			So(fields, ShouldResemble, []string{"fieldA", "fieldB", "boolean", "this", "this"})
		})
		Convey("csv values are printed correctly for a given sample struct", func() {
			s := SampleStruct{
				FieldA:    "hello",
				FieldInt:  1,
				FieldBool: true,
				Sub1:      SampleSubStruct{"this", "print"},
				Sub2:      SampleSubStruct{"another", "no"},
				NoCSV:     "hi",
			}
			values := getCSVValues(s)
			So(values, ShouldResemble, []string{"hello", "1", "true", "this", "another"})
		})
	})
}

func TestConvertCSVToRecord(t *testing.T) {
	Convey("with a list of sample structs", t, func() {
		s := SampleStruct{
			FieldA:    "hello",
			FieldInt:  1,
			FieldBool: true,
			Sub1:      SampleSubStruct{"this", "print"},
			Sub2:      SampleSubStruct{"another", "no"},
			NoCSV:     "hi",
		}
		s2 := SampleStruct{
			FieldA:    "world",
			FieldInt:  1,
			FieldBool: true,
			Sub1:      SampleSubStruct{"mongo", "db"},
			Sub2:      SampleSubStruct{"ever", "green"},
			NoCSV:     "hi",
		}
		samples := []SampleStruct{s, s2}
		Convey("passing in a struct to convert to a record that is not a list errors", func() {
			_, err := convertDataToCSVRecord(s)
			So(err, ShouldNotBeNil)
		})
		Convey("passing in a valid set of structs should convert the record", func() {
			strings, err := convertDataToCSVRecord(samples)
			So(err, ShouldBeNil)
			So(len(strings), ShouldEqual, 3)
			So(strings[0], ShouldResemble, []string{"fieldA", "fieldB", "boolean", "this", "this"})
			So(strings[1], ShouldResemble, []string{"hello", "1", "true", "this", "another"})
			So(strings[2], ShouldResemble, []string{"world", "1", "true", "mongo", "ever"})
		})
	})
}
