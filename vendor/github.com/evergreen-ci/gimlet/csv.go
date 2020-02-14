package gimlet

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"reflect"

	"github.com/pkg/errors"
)

// getCSVFields takes in a struct and retrieves the struct tag values for csv.
// If there is a substruct, then it will recurse and flatten out those fields.
func getCSVFields(t reflect.Type) []string {
	fields := []string{}
	numberFields := t.NumField()
	for i := 0; i < numberFields; i++ {
		fieldType := t.Field(i).Type
		if fieldType.Kind() == reflect.Struct && fieldType.NumField() > 0 {
			fields = append(fields, getCSVFields(fieldType)...)
			continue
		}
		stringVal := t.Field(i).Tag.Get("csv")
		if stringVal != "" {
			fields = append(fields, stringVal)
		}
	}
	return fields
}

// getCSVValues takes in a struct and returns a string of values based on struct tags
func getCSVValues(data interface{}) []string {
	values := []string{}
	v := reflect.ValueOf(data)
	t := reflect.TypeOf(data)
	numberFields := v.NumField()
	for i := 0; i < numberFields; i++ {
		fieldType := v.Field(i).Type()
		if fieldType.Kind() == reflect.Struct && fieldType.NumField() > 0 {
			values = append(values, getCSVValues(v.Field(i).Interface())...)
			continue
		}
		stringVal := t.Field(i).Tag.Get("csv")
		if stringVal != "" {
			values = append(values, fmt.Sprintf("%v", v.Field(i).Interface()))
		}
	}
	return values
}

// convertDataToCSVRecord takes in an interface that is a slice or an array and converts the struct to csv for
// fields that have the the csv struct tag.
func convertDataToCSVRecord(data interface{}) ([][]string, error) {
	switch reflect.TypeOf(data).Kind() {
	case reflect.Slice, reflect.Array:
		s := reflect.ValueOf(data)
		if s.Len() == 0 {
			return nil, errors.New("no data to write to CSV")
		}
		// create the fields by passing in the type of a host utilization bucket
		records := [][]string{getCSVFields(reflect.TypeOf(s.Index(0).Interface()))}
		for i := 0; i < s.Len(); i++ {
			records = append(records, getCSVValues(s.Index(i).Interface()))
		}
		return records, nil
	default:
		return nil, errors.New("data is not an array")
	}
}

// WriteToCSVResponse takes in an interface that is a slice or an array and converts the struct to csv for
// fields that have the the csv struct tag.
func WriteCSVResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Add("Connection", "close")

	csvRecords, err := convertDataToCSVRecord(data)
	if err != nil {
		var err2 error
		csvRecords, err2 = convertDataToCSVRecord([]ErrorResponse{
			{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			},
		})

		if err2 != nil {
			panic(fmt.Sprintf("%+v: %+v", err, err2))
		}

		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(status)
	}

	csvWriter := csv.NewWriter(w)
	if err = csvWriter.WriteAll(csvRecords); err != nil {
		panic(err)
	}
}

// WriteCSV is a helper function to write arrays of data in CSV
// format to a response body. Use this to write the data with a
func WriteCSV(w http.ResponseWriter, data interface{}) {
	// 200
	WriteCSVResponse(w, http.StatusOK, data)
}

func WriteCSVError(w http.ResponseWriter, data interface{}) {
	// 400
	WriteCSVResponse(w, http.StatusBadRequest, data)
}

func WriteCSVInternalError(w http.ResponseWriter, data interface{}) {
	// 500
	WriteCSVResponse(w, http.StatusInternalServerError, data)
}
