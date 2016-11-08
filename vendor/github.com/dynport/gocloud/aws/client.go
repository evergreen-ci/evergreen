package aws

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"hash"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

var (
	Debug                  = os.Getenv("DEBUG") == "true"
	b64                    = base64.StdEncoding
	metadataIp             = "169.254.169.254"
	securityCredentialsUrl = "http://" + metadataIp + "/latest/meta-data/iam/security-credentials/"
)

type Client struct {
	Key, Secret, Region, SecurityToken string
}

func NewFromEnv() *Client {
	client := &Client{
		Key:    os.Getenv(ENV_AWS_ACCESS_KEY),
		Secret: os.Getenv(ENV_AWS_SECRET_KEY),
		Region: os.Getenv(ENV_AWS_DEFAULT_REGION),
	}

	if client.Key == "" || client.Secret == "" {
		var e error
		client, e = newFromIam()
		if e != nil {
			abortWith(fmt.Sprintf("%s and %s must be set in ENV", ENV_AWS_ACCESS_KEY, ENV_AWS_SECRET_KEY))
		}
	}
	return client
}

type Response struct {
	StatusCode int
	Content    []byte
}

func QueryPrefix(version, action string) string {
	return "Version=" + version + "&Action=" + action
}

type ErrorResponse struct {
	XMLName   xml.Name `xml:"ErrorResponse"`
	RequestID string   `xml:"RequestID"`
	Errors    []*Error `xml:"Error"`
}

type Error struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

type ErrorsResponse struct {
	XMLName   xml.Name `xml:"Response"`
	RequestID string   `xml:"RequestID"`
	Errors    []*Error `xml:"Errors>Error"`
}

func errorsString(errors []*Error) string {
	out := []string{}
	for _, e := range errors {
		out = append(out, fmt.Sprintf("%s: %s", e.Code, e.Message))
	}
	return strings.Join(out, ", ")
}

func ExtractError(b []byte) error {
	rsp := &ErrorResponse{}
	e := xml.Unmarshal(b, rsp)
	if e == nil {
		return fmt.Errorf(errorsString(rsp.Errors))
	}
	return ExtractErrorsResponse(b)
}

func ExtractErrorsResponse(b []byte) error {
	rsp := &ErrorsResponse{}
	e := xml.Unmarshal(b, rsp)
	if e == nil {
		return fmt.Errorf(errorsString(rsp.Errors))
	}
	return nil
}

// list of endpoints
func (client *Client) DoSignedRequest(method string, endpoint, action string, extraAttributes map[string]string) (rsp *Response, e error) {
	url := endpoint + "?" + action
	dbg.Printf("request url=%q with method=%q", url, method)
	request, e := http.NewRequest(method, url, nil)
	client.SignAwsRequestV2(request, time.Now())
	raw, e := http.DefaultClient.Do(request)
	if e != nil {
		return rsp, e
	}
	defer raw.Body.Close()
	dbg.Printf("got response with status=%q", raw.Status)
	rsp = &Response{
		StatusCode: raw.StatusCode,
	}
	rsp.Content, e = ioutil.ReadAll(raw.Body)
	if e != nil {
		return rsp, e
	}
	dbg.Printf("and content")
	dbg.Print(string(rsp.Content))
	e = ExtractError(rsp.Content)
	if e != nil {
		return nil, e
	}
	return rsp, e
}

func (client *Client) signPayload(payload string, hash func() hash.Hash) string {
	h := hmac.New(hash, []byte(client.Secret))
	h.Write([]byte(payload))
	signature := make([]byte, b64.EncodedLen(h.Size()))
	b64.Encode(signature, h.Sum(nil))
	return string(signature)
}

func (client *Client) SignAwsRequest(req *http.Request) {
	date := time.Now().UTC().Format(http.TimeFormat)
	token := "AWS3-HTTPS AWSAccessKeyId=" + client.Key + ",Algorithm=HmacSHA256,Signature=" + client.signPayload(date, sha256.New)
	req.Header.Set("X-Amzn-Authorization", token)
	req.Header.Set("x-amz-date", date)
	return
}

func timestamp(t time.Time) string {
	return strings.TrimSuffix(t.UTC().Format(time.RFC3339), "Z")
}

func (client *Client) v2PayloadAndQuery(req *http.Request, refTime time.Time) (payload, rawQuery string) {
	values := req.URL.Query()
	if len(values["AWSAccessKeyId"]) == 0 {
		values.Add("AWSAccessKeyId", client.Key)
	}

	if len(values["SignatureVersion"]) == 0 {
		values.Add("SignatureVersion", "2")
	}

	if len(values["SignatureMethod"]) == 0 {
		values.Add("SignatureMethod", "HmacSHA256")
	}

	if len(values["Timestamp"]) == 0 {
		values.Add("Timestamp", timestamp(refTime))
	}

	var keys, sarray []string
	for k, _ := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sarray = append(sarray, Encode(k)+"="+Encode(values[k][0]))
	}

	joined := strings.Join(sarray, "&")
	path := "/"
	if req.URL.Path != "" {
		path = req.URL.Path
	}

	return strings.Join([]string{
		req.Method,
		req.URL.Host,
		path,
		joined,
	}, "\n"), joined
}

func (client *Client) SignAwsRequestV2(req *http.Request, t time.Time) {
	values := req.URL.Query()
	if len(values["Timestamp"]) == 0 {
		values.Add("Timestamp", timestamp(t))
	}
	if client.SecurityToken != "" {
		values.Add("SecurityToken", client.SecurityToken)
	}
	req.URL.RawQuery = values.Encode()
	payload, query := client.v2PayloadAndQuery(req, t)
	query += "&Signature=" + Encode(client.signPayload(payload, sha256.New))
	req.URL.RawQuery = query
}

// authentication:	http://s3.amazonaws.com/doc/s3-developer-guide/RESTAuthentication.html
// upload:			http://docs.aws.amazon.com/AmazonS3/latest/API/RESTObjectPUT.html
func (client *Client) SignS3Request(req *http.Request) {
	payload := s3Payload(req)
	req.Header.Add("Authorization", "AWS "+client.Key+":"+client.signPayload(payload, sha1.New))
}

func s3Payload(req *http.Request) string {
	date := req.Header.Get("Date")
	if date == "" {
		date = time.Now().Format(http.TimeFormat)
		req.Header.Set("Date", date)
	}
	payloadParts := []string{
		req.Method,
		req.Header.Get(CONTENT_MD5),
		req.Header.Get(CONTENT_TYPE),
		date,
	}
	amzHeaders := []string{}
	for k, v := range req.Header {
		value := strings.ToLower(k) + ":" + strings.Join(v, ",")
		if strings.HasPrefix(value, "x-amz") {
			amzHeaders = append(amzHeaders, value)
		}
	}
	sort.Strings(amzHeaders)
	payloadParts = append(payloadParts, amzHeaders...)
	payloadParts = append(payloadParts, req.URL.Path)
	return strings.Join(payloadParts, "\n")
}
