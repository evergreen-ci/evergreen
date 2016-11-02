package util

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"reflect"
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
			return nil, fmt.Errorf("no data to write to CSV")
		}
		// create the fields by passing in the type of a host utilization bucket
		records := [][]string{getCSVFields(reflect.TypeOf(s.Index(0).Interface()))}
		for i := 0; i < s.Len(); i++ {
			records = append(records, getCSVValues(s.Index(i).Interface()))
		}
		return records, nil
	default:
		return nil, fmt.Errorf("data is not an array")
	}
}

// WriteToCSVResponse takes in an interface that is a slice or an array and converts the struct to csv for
// fields that have the the csv struct tag.
func WriteCSVResponse(w http.ResponseWriter, status int, data interface{}) {
	// if the status is not okay then don't convert the data to csv.
	if status != http.StatusOK {
		bytes := []byte(fmt.Sprintf("%v", data))
		w.WriteHeader(500)
		w.Write(bytes)
		return
	}

	w.Header().Add("Content-Type", "application/csv")
	w.Header().Add("Connection", "close")

	csvRecords, err := convertDataToCSVRecord(data)

	if err != nil {
		stringBytes := []byte(err.Error())
		w.WriteHeader(500)
		w.Write(stringBytes)
		return
	}
	w.WriteHeader(http.StatusOK)
	csvWriter := csv.NewWriter(w)
	csvWriter.WriteAll(csvRecords)
	return
}
