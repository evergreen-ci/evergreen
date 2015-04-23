package render

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
)

const expected = `<html><body><div>menu</div><p>hello Socrates setarcoS</p></body></html>`

var testData = struct {
	Name string
}{"Socrates"}

var funcMap = template.FuncMap{
	"Reverse": func(in string) string {
		runes := []rune(in)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes)
	},
}

func TestHTML(t *testing.T) {
	r := New(Options{
		Directory: "testdata",
		Funcs:     funcMap,
	})
	out := &bytes.Buffer{}
	err := r.HTML(out, testData, "base", "test1.html", "test2.html")

	if err != nil {
		t.Errorf("Got from HTML(): %v", err)
	}

	if string(out.Bytes()) != expected {
		t.Errorf("Expected: [%v]\ngot : [%v]", expected, string(out.Bytes()))
	}
}

func TestBadTemplates(t *testing.T) {
	rndr := New(Options{
		Directory: "testdata",
		Funcs:     funcMap,
	})

	out := &bytes.Buffer{}
	err := rndr.HTML(out, testData, "base", "invalid_template.html")
	if err == nil {
		t.Errorf("expected invalid template file to trigger err on parse (but it didn't)")
	}

	err = rndr.HTML(out, testData, "base", "template_does_not_exist.html")
	if err == nil {
		t.Errorf("expected nonexistent template file to trigger err on load (but it didn't)")
	}

	err = rndr.HTML(out, testData, "base", "badtemplate.html")
	if err == nil {
		t.Errorf("expected template execution to return err but it did not")
	}

	req, err := http.NewRequest("GET", "http://example.com/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	htmlHandler := func(w http.ResponseWriter, r *http.Request) {
		rndr.WriteHTML(w, http.StatusOK, testData, "badtemplate.html")
	}

	w := httptest.NewRecorder()
	htmlHandler(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected Internal Server Error (500) but got %v", w.Code)
	}

}

func TestWriteHTTP(t *testing.T) {
	rndr := New(Options{
		Directory: "testdata",
		Funcs:     funcMap,
	})
	req, err := http.NewRequest("GET", "http://example.com/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	/* Test a handler that writes a rendered HTML template */
	w := httptest.NewRecorder()
	htmlHandler := func(w http.ResponseWriter, r *http.Request) {
		rndr.WriteHTML(w, http.StatusOK, testData, "base", "test1.html", "test2.html")
	}
	htmlHandler(w, req)
	if string(w.Body.Bytes()) != expected {
		t.Errorf("Expected: [%v]\ngot : [%v]", expected, string(w.Body.Bytes()))
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected OK (200) but got %v", w.Code)
	}

	/* Test a handler that marshals and writes JSON */
	w = httptest.NewRecorder()
	jsonHandler := func(w http.ResponseWriter, r *http.Request) {
		rndr.WriteJSON(w, http.StatusOK, testData)
	}
	jsonHandler(w, req)

	/* parse the json and check the fields */
	parsed := map[string]string{}
	err = json.Unmarshal(w.Body.Bytes(), &parsed)
	if parsed["Name"] != testData.Name {
		t.Errorf("serialized data does not match expected test data")
	}
}
