package rehttp

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryHTTPMethods(t *testing.T) {
	cases := []struct {
		retries int
		meths   []string
		inMeth  string
		att     int
		want    bool
	}{
		{retries: 1, meths: nil, inMeth: "GET", att: 0, want: false},
		{retries: 0, meths: nil, inMeth: "GET", att: 1, want: false},
		{retries: 1, meths: []string{"get"}, inMeth: "GET", att: 0, want: true},
		{retries: 1, meths: []string{"GET"}, inMeth: "GET", att: 0, want: true},
		{retries: 1, meths: []string{"GET"}, inMeth: "POST", att: 0, want: false},
		{retries: 2, meths: []string{"GET", "POST"}, inMeth: "POST", att: 0, want: true},
		{retries: 2, meths: []string{"GET", "POST"}, inMeth: "POST", att: 1, want: true},
		{retries: 2, meths: []string{"GET", "POST"}, inMeth: "POST", att: 2, want: false},
		{retries: 2, meths: []string{"GET", "POST"}, inMeth: "put", att: 0, want: false},
		{retries: 2, meths: []string{"GET", "POST", "PUT"}, inMeth: "put", att: 0, want: true},
	}

	for i, tc := range cases {
		fn := RetryAll(RetryMaxRetries(tc.retries), RetryHTTPMethods(tc.meths...))
		req, err := http.NewRequest(tc.inMeth, "", nil)
		require.Nil(t, err)
		got := fn(Attempt{Request: req, Index: tc.att})
		assert.Equal(t, tc.want, got, "%d", i)
	}
}

func TestRetryStatusInterval(t *testing.T) {
	cases := []struct {
		retries int
		res     *http.Response
		att     int
		want    bool
	}{
		{retries: 1, res: nil, att: 0, want: false},
		{retries: 1, res: nil, att: 1, want: false},
		{retries: 1, res: &http.Response{StatusCode: 200}, att: 0, want: false},
		{retries: 1, res: &http.Response{StatusCode: 400}, att: 0, want: false},
		{retries: 1, res: &http.Response{StatusCode: 500}, att: 0, want: true},
		{retries: 1, res: &http.Response{StatusCode: 500}, att: 1, want: false},
		{retries: 2, res: &http.Response{StatusCode: 599}, att: 0, want: true},
		{retries: 2, res: &http.Response{StatusCode: 503}, att: 1, want: true},
		{retries: 2, res: &http.Response{StatusCode: 500}, att: 2, want: false},
		{retries: 2, res: &http.Response{StatusCode: 600}, att: 0, want: false},
	}

	for i, tc := range cases {
		fn := RetryAll(RetryMaxRetries(tc.retries), RetryStatusInterval(500, 600))
		got := fn(Attempt{Response: tc.res, Index: tc.att})
		assert.Equal(t, tc.want, got, "%d", i)
	}
}

func TestRetryStatuses(t *testing.T) {
	cases := []struct {
		retries int
		res     *http.Response
		att     int
		want    bool
	}{
		{retries: 1, res: nil, att: 0, want: false},
		{retries: 1, res: nil, att: 1, want: false},
		{retries: 1, res: &http.Response{StatusCode: 200}, att: 0, want: false},
		{retries: 1, res: &http.Response{StatusCode: 400}, att: 0, want: false},
		{retries: 1, res: &http.Response{StatusCode: 401}, att: 0, want: true},
		{retries: 1, res: &http.Response{StatusCode: 401}, att: 1, want: false},
		{retries: 1, res: &http.Response{StatusCode: 402}, att: 0, want: false},
		{retries: 1, res: &http.Response{StatusCode: 500}, att: 0, want: true},
		{retries: 1, res: &http.Response{StatusCode: 500}, att: 1, want: false},
		{retries: 2, res: &http.Response{StatusCode: 500}, att: 0, want: true},
		{retries: 2, res: &http.Response{StatusCode: 500}, att: 1, want: true},
		{retries: 2, res: &http.Response{StatusCode: 500}, att: 2, want: false},
	}

	for i, tc := range cases {
		fn := RetryAll(RetryMaxRetries(tc.retries), RetryStatuses(401, 500))
		got := fn(Attempt{Response: tc.res, Index: tc.att})
		assert.Equal(t, tc.want, got, "%d", i)
	}
}

type tempErr struct{}

func (t tempErr) Error() string   { return "temp error" }
func (t tempErr) Temporary() bool { return true }

func TestRetryTemporaryErr(t *testing.T) {
	cases := []struct {
		retries int
		err     error
		att     int
		want    bool
	}{
		{retries: 1, err: nil, att: 0, want: false},
		{retries: 1, err: nil, att: 1, want: false},
		{retries: 1, err: io.EOF, att: 0, want: false},
		{retries: 1, err: tempErr{}, att: 0, want: true},
		{retries: 1, err: tempErr{}, att: 1, want: false},
	}

	for i, tc := range cases {
		fn := RetryAll(RetryMaxRetries(tc.retries), RetryTemporaryErr())
		got := fn(Attempt{Index: tc.att, Error: tc.err})
		assert.Equal(t, tc.want, got, "%d", i)
	}
}

func TestRetryAll(t *testing.T) {
	max := RetryMaxRetries(2)
	status := RetryStatusInterval(500, 600)
	temp := RetryTemporaryErr()
	meths := RetryHTTPMethods("GET")
	fn := RetryAll(max, status, temp, meths)

	cases := []struct {
		method string
		status int
		att    int
		err    error
		want   bool
	}{
		{"POST", 200, 0, nil, false},
		{"GET", 200, 0, nil, false},
		{"GET", 500, 0, nil, false},
		{"GET", 500, 0, tempErr{}, true},
		{"GET", 500, 1, tempErr{}, true},
		{"GET", 500, 2, tempErr{}, false},
		{"GET", 400, 0, tempErr{}, false},
		{"GET", 500, 0, io.EOF, false},
	}
	for i, tc := range cases {
		got := fn(Attempt{
			Request:  &http.Request{Method: tc.method},
			Response: &http.Response{StatusCode: tc.status},
			Index:    tc.att,
			Error:    tc.err,
		})
		assert.Equal(t, tc.want, got, "%d", i)
	}

	// en empty RetryAll always returns true
	fn = RetryAll()
	got := fn(Attempt{Index: 0})
	assert.True(t, got, "empty RetryAll")
}

func TestRetryAny(t *testing.T) {
	max := RetryMaxRetries(2)
	status := RetryStatusInterval(500, 600)
	temp := RetryTemporaryErr()
	meths := RetryHTTPMethods("GET")
	fn := RetryAny(status, temp, meths)
	fn = RetryAll(max, fn)

	cases := []struct {
		method string
		status int
		att    int
		err    error
		want   bool
	}{
		{"POST", 200, 0, nil, false},
		{"GET", 200, 0, nil, true},
		{"POST", 500, 0, nil, true},
		{"POST", 200, 0, tempErr{}, true},
		{"POST", 200, 0, io.EOF, false},
		{"GET", 500, 0, tempErr{}, true},
		{"GET", 500, 1, tempErr{}, true},
		{"GET", 500, 2, tempErr{}, false},
	}
	for i, tc := range cases {
		got := fn(Attempt{
			Request:  &http.Request{Method: tc.method},
			Response: &http.Response{StatusCode: tc.status},
			Index:    tc.att,
			Error:    tc.err,
		})
		assert.Equal(t, tc.want, got, "%d", i)
	}

	// en empty RetryAny always returns false
	fn = RetryAny()
	got := fn(Attempt{Index: 0})
	assert.False(t, got, "empty RetryAny")
}

func TestToRetryFn(t *testing.T) {
	fn := toRetryFn(RetryAll(RetryMaxRetries(2), RetryTemporaryErr()), ConstDelay(time.Second))

	cases := []struct {
		err       error
		att       int
		wantRetry bool
		wantDelay time.Duration
	}{
		{err: nil, att: 0, wantRetry: false, wantDelay: 0},
		{err: io.EOF, att: 0, wantRetry: false, wantDelay: 0},
		{err: tempErr{}, att: 0, wantRetry: true, wantDelay: time.Second},
		{err: tempErr{}, att: 1, wantRetry: true, wantDelay: time.Second},
		{err: tempErr{}, att: 2, wantRetry: false, wantDelay: 0},
	}

	for i, tc := range cases {
		retry, delay := fn(Attempt{Index: tc.att, Error: tc.err})
		assert.Equal(t, tc.wantRetry, retry, "%d - retry?", i)
		assert.Equal(t, tc.wantDelay, delay, "%d - delay", i)
	}
}

func TestCombineRetryFn(t *testing.T) {
	// retry if:
	// - attempt index < 2
	// - AND
	//   - (GET OR status 401 or 403)
	//   - OR
	//   - (POST AND status 500)
	any := RetryAny(RetryHTTPMethods("GET"), RetryStatuses(401, 403))
	all := RetryAll(RetryHTTPMethods("POST"), RetryStatuses(500))
	fn := RetryAll(RetryMaxRetries(2), RetryAny(any, all))

	cases := []struct {
		att    int
		meth   string
		status int
		want   bool
	}{
		{0, "DELETE", 200, false},
		{0, "DELETE", 401, true},
		{0, "DELETE", 500, false},
		{0, "GET", 400, true},
		{0, "GET", 401, true},
		{0, "GET", 403, true},
		{1, "GET", 403, true},
		{2, "GET", 403, false},
		{0, "POST", 403, true},
		{0, "POST", 404, false},
		{0, "POST", 500, true},
		{1, "POST", 500, true},
		{2, "POST", 500, false},
	}
	for i, c := range cases {
		got := fn(Attempt{
			Index:    c.att,
			Request:  &http.Request{Method: c.meth},
			Response: &http.Response{StatusCode: c.status},
		})
		assert.Equal(t, c.want, got, "%d", i)
	}
}
