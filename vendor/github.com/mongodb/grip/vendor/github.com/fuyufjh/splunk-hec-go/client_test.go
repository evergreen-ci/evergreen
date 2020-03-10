package hec

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	testSplunkURL   = "http://localhost:8088"
	testSplunkToken = "00000000-0000-0000-0000-000000000000"
)

var testHttpClient *http.Client = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
	Timeout: 100 * time.Millisecond,
}

func jsonEndpoint(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failed := false
		input := make(map[string]interface{})
		j := json.NewDecoder(r.Body)
		err := j.Decode(&input)
		if err != nil {
			t.Errorf("Decoding JSON: %v", err)
			failed = true
		}

		requiredFields := []string{"event"}
		allowedFields := map[string]struct{}{
			"channel":    struct{}{},
			"event":      struct{}{},
			"fields":     struct{}{},
			"host":       struct{}{},
			"index":      struct{}{},
			"source":     struct{}{},
			"sourcetype": struct{}{},
			"time":       struct{}{},
		}
		for _, f := range requiredFields {
			if _, ok := input[f]; !ok {
				t.Errorf("Required field %q missing in %v", f, input)
			}
		}
		for f := range input {
			if _, ok := allowedFields[f]; !ok {
				t.Errorf("Unexpected field %q in %v", f, input)
			}
		}
		if failed {
			w.WriteHeader(400)
			w.Write([]byte(`{"text": "Error processing event", "code": 90}`))
		} else {
			w.Write([]byte(`{"text":"Success","code":0}`))
		}
	})
}

func TestHEC_WriteEvent(t *testing.T) {
	event := &Event{
		Index:      String("main"),
		Source:     String("test-hec-raw"),
		SourceType: String("manual"),
		Host:       String("localhost"),
		Time:       String("1485237827.123"),
		Event:      "hello, world",
	}

	ts := httptest.NewServer(jsonEndpoint(t))
	c := NewClient(ts.URL, testSplunkToken)
	c.SetHTTPClient(testHttpClient)
	err := c.WriteEvent(event)
	assert.NoError(t, err)
}

func TestHEC_WriteEventServerFailure(t *testing.T) {
	event := &Event{
		Index:      String("main"),
		Source:     String("test-hec-raw"),
		SourceType: String("manual"),
		Host:       String("localhost"),
		Time:       String("1485237827.123"),
		Event:      "hello, world",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"text":"Data channel is missing","code":10}`))
	}))
	c := NewClient(ts.URL, testSplunkToken)
	c.SetHTTPClient(testHttpClient)
	err := c.WriteEvent(event)
	assert.Error(t, err)
}

func TestHEC_WriteObjectEvent(t *testing.T) {
	event := &Event{
		Index:      String("main"),
		Source:     String("test-hec-raw"),
		SourceType: String("manual"),
		Host:       String("localhost"),
		Time:       String("1485237827.123"),
		Event: map[string]interface{}{
			"str":  "hello",
			"time": time.Now(),
		},
	}

	ts := httptest.NewServer(jsonEndpoint(t))
	c := NewClient(ts.URL, testSplunkToken)
	c.SetHTTPClient(testHttpClient)
	err := c.WriteEvent(event)
	assert.NoError(t, err)
}

func TestHEC_WriteLongEvent(t *testing.T) {
	event := &Event{
		Index:      String("main"),
		Source:     String("test-hec-raw"),
		SourceType: String("manual"),
		Host:       String("localhost"),
		Time:       String("1485237827.123"),
		Event:      "hello, world",
	}

	ts := httptest.NewServer(jsonEndpoint(t))
	c := NewClient(ts.URL, testSplunkToken)

	c.SetHTTPClient(testHttpClient)
	c.SetMaxContentLength(20) // less than full event
	err := c.WriteEvent(event)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "too long")
}

func TestHEC_WriteEventBatch(t *testing.T) {
	events := []*Event{
		{Event: "event one"},
		{Event: "event two"},
	}

	ts := httptest.NewServer(jsonEndpoint(t))
	c := NewClient(ts.URL, testSplunkToken)

	c.SetHTTPClient(testHttpClient)
	err := c.WriteBatch(events)
	assert.NoError(t, err)
}

func TestHEC_WriteLongEventBatch(t *testing.T) {
	events := []*Event{
		{Event: "event one"},
		{Event: "event two"},
	}

	ts := httptest.NewServer(jsonEndpoint(t))
	c := NewClient(ts.URL, testSplunkToken)
	c.SetHTTPClient(testHttpClient)
	c.SetMaxContentLength(25)
	err := c.WriteBatch(events)
	assert.NoError(t, err)
}

func TestHEC_WriteEventRaw(t *testing.T) {
	events := `2017-01-24T06:07:10.488Z Raw event one
2017-01-24T06:07:12.434Z Raw event two`
	metadata := EventMetadata{
		Source: String("test-hec-raw"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"text":"Success","code":0}`))
	}))
	c := NewClient(ts.URL, testSplunkToken)
	c.SetHTTPClient(testHttpClient)
	err := c.WriteRaw(strings.NewReader(events), &metadata)
	assert.NoError(t, err)
}

func TestHEC_WriteLongEventRaw(t *testing.T) {
	events := `2017-01-24T06:07:10.488Z Raw event one
2017-01-24T06:07:12.434Z Raw event two`
	metadata := EventMetadata{
		Source: String("test-hec-raw"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"text":"Success","code":0}`))
	}))
	c := NewClient(ts.URL, testSplunkToken)
	c.SetMaxContentLength(40)
	c.SetHTTPClient(testHttpClient)
	err := c.WriteRaw(strings.NewReader(events), &metadata)
	assert.NoError(t, err)
}

func TestHEC_WriteRawFailure(t *testing.T) {
	events := `2017-01-24T06:07:10.488Z Raw event one
2017-01-24T06:07:12.434Z Raw event two`
	metadata := EventMetadata{
		Source: String("test-hec-raw"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"text":"Oh no","code":90}`))
	}))
	c := NewClient(ts.URL, testSplunkToken)
	c.SetMaxContentLength(40)
	c.SetHTTPClient(testHttpClient)
	err := c.WriteRaw(strings.NewReader(events), &metadata)
	assert.Error(t, err)
}

func TestBreakStream(t *testing.T) {
	text := "This is line A\nThis is line B" // length of every line is 14

	getCountFunc := func(counter *int) func(chunk []byte) error {
		// returned function adds count of all character except "\n"
		return func(chunk []byte) error {
			for _, b := range chunk {
				if b != '\n' {
					*counter++
				}
			}
			return nil
		}
	}

	for _, max := range []int{13, 14, 15, 28, 5, 30} {
		var counter int = 0
		err := breakStream(strings.NewReader(text), max, getCountFunc(&counter))
		assert.NoError(t, err)
		assert.Equal(t, 28, counter)
	}
}
