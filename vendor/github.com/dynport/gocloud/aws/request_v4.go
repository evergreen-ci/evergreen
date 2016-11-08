package aws

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	algorithm              = "AWS4-HMAC-SHA256"
	timeFormat             = "20060102T150405Z"
	dateFormat             = "20060102"
	streamingPayloadSha256 = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD"
)

type RequestV4 struct {
	Key    string
	Secret string

	Method       string
	URL          *url.URL
	Payload      []byte
	Region       string
	Service      string
	Time         time.Time
	cachedValues url.Values

	header    http.Header
	validated bool
}

func (r *RequestV4) Request() (*http.Request, error) {
	headers, e := r.validatedHeaders()
	if e != nil {
		return nil, e
	}
	var req *http.Request
	if len(r.Payload) > 0 {
		req, e = http.NewRequest(r.Method, r.URL.String(), bytes.NewBuffer(r.Payload))
	} else {
		req, e = http.NewRequest(r.Method, r.URL.String(), nil)
	}
	if e != nil {
		return nil, e
	}

	for k, v := range headers {
		for _, value := range v {
			if req.Header.Get(k) == "" {
				req.Header.Set(k, value)
			}
		}
	}
	auth, e := r.authorization()
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", auth)
	return req, nil
}

func (r *RequestV4) SetHeader(key, value string) {
	if r.header == nil {
		r.header = http.Header{}
	}
	r.header.Set(key, value)
}

func (r *RequestV4) validatedHeaders() (http.Header, error) {
	if r.validated {
		return r.header, nil
	}
	if r.header == nil {
		r.header = http.Header{}
	}
	errors := []string{}

	m := map[string]string{
		"Service": r.Service,
		"Region":  r.Region,
		"Key":     r.Key,
		"Secret":  r.Secret,
	}
	for k, v := range m {
		if v == "" {
			errors = append(errors, k)
		}
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("validating request: %s must be set", strings.Join(errors, ", "))
	}

	if r.header.Get("x-amz-date") == "" {
		r.header.Set("x-amz-date", r.time().Format(timeFormat))
	}
	if r.header.Get("host") == "" {
		r.header.Set("host", r.URL.Host)
	}
	r.validated = true
	return r.header, nil
}

func (r *RequestV4) stringToSign() (string, error) {
	hashedCanonicalRequest, e := r.hashedCanonicalRequest()
	if e != nil {
		return "", e
	}
	return strings.Join([]string{
		algorithm,
		r.time().Format(timeFormat),
		r.credentialScopeValue(),
		hashedCanonicalRequest,
	}, "\n"), nil
}

func (r *RequestV4) authorization() (string, error) {
	_, signedHeaders, e := r.canonicalHeadersAndValues()
	if e != nil {
		return "", e
	}
	sig, e := r.signature()
	if e != nil {
		return "", e
	}
	return algorithm + " Credential=" + strings.Join(
		[]string{
			r.Key + "/" + r.credentialScopeValue(),
			"SignedHeaders=" + signedHeaders,
			"Signature=" + sig,
		}, ", ",
	), nil
}

func (r *RequestV4) credentialScopeValue() string {
	return r.time().Format(dateFormat) + "/" + r.Region + "/" + r.Service + "/aws4_request"
}

func (r *RequestV4) signature() (string, error) {
	sts, e := r.stringToSign()
	if e != nil {
		return "", e
	}
	x := hmacDigest(
		hmacDigest(
			hmacDigest(
				hmacDigest(
					hmacDigest(
						[]byte("AWS4"+r.Secret),
						r.time().Format(dateFormat),
					), r.Region,
				), r.Service,
			),
			"aws4_request",
		),
		sts,
	)
	return fmt.Sprintf("%x", x), nil
}

func hmacDigest(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func (r *RequestV4) time() time.Time {
	if r.Time.IsZero() {
		r.Time = time.Now()
	}
	return r.Time.UTC()
}

func (r *RequestV4) values() url.Values {
	if r.cachedValues == nil {
		r.cachedValues = r.URL.Query()
	}
	return r.cachedValues
}

func (r *RequestV4) canonicalURI() string {
	path := "/"
	if r.URL.Path != "" {
		path = r.URL.Path
	}
	return path
}

func (r *RequestV4) hashedPayload() string {
	if len(r.Payload) == 0 {
		return streamingPayloadSha256
	}
	return sha256Digest(string(r.Payload))
}

func (r *RequestV4) hashedCanonicalRequest() (string, error) {
	out, e := r.canonicalForm()
	if e != nil {
		return "", e
	}
	return sha256Digest(out), nil
}

func sha256Digest(s string) string {
	hash := sha256.New()
	hash.Write([]byte(s))
	return fmt.Sprintf("%x", hash.Sum(nil))

}

func (r *RequestV4) canonicalForm() (string, error) {
	canonicalHeaders, signedHeaders, e := r.canonicalHeadersAndValues()
	if e != nil {
		return "", e
	}
	return strings.Join([]string{
		r.Method,
		r.canonicalURI(),
		r.canonicalQueryString(),
		canonicalHeaders,
		signedHeaders,
		r.hashedPayload(),
	}, "\n"), nil
}

func (r *RequestV4) canonicalQueryString() string {
	var keys, sarray []string
	for k, _ := range r.values() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sarray = append(sarray, Encode(k)+"="+Encode(r.values()[k][0]))
	}
	return strings.Join(sarray, "&")
}

func (r *RequestV4) canonicalHeaders() (string, error) {
	out, _, e := r.canonicalHeadersAndValues()
	return out, e
}

func (r *RequestV4) signedHeaders() (string, error) {
	_, out, e := r.canonicalHeadersAndValues()
	return out, e
}

func (r *RequestV4) canonicalHeadersAndValues() (string, string, error) {
	amzHeaders := []string{}
	names := []string{}
	headers, e := r.validatedHeaders()
	if e != nil {
		return "", "", e
	}
	for k, v := range headers {
		h := strings.TrimSpace(strings.Join(v, ","))
		if !strings.HasPrefix(h, `"`) || !strings.HasSuffix(h, `"`) {
			h = strings.Join(strings.Fields(h), " ")
		}
		headerName := strings.ToLower(k)
		value := headerName + ":" + h
		names = append(names, headerName)
		amzHeaders = append(amzHeaders, value)
	}
	sort.Strings(amzHeaders)
	sort.Strings(names)
	return strings.Join(amzHeaders, "\n") + "\n", strings.Join(names, ";"), nil
}
