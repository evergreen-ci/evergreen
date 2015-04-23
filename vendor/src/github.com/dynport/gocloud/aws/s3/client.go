package s3

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/dynport/gocloud/aws"
)

var b64 = base64.StdEncoding

const (
	DEFAULT_ENDPOINT_HOST         = "s3.amazonaws.com"
	HEADER_CONTENT_MD5            = "Content-Md5"
	HEADER_CONTENT_TYPE           = "Content-Type"
	HEADER_DATE                   = "Date"
	HEADER_AUTHORIZATION          = "Authorization"
	AMZ_ACL_PUBLIC                = "public-read"
	DEFAULT_CONTENT_TYPE          = "application/octet-stream"
	HEADER_AMZ_ACL                = "x-amz-acl"
	HEADER_SERVER_SIDE_ENCRUPTION = "x-amz-server-side-encryption"
	AES256                        = "AES256"
)

type Client struct {
	*aws.Client
	CustomEndpointHost string
	UseSsl             bool
}

func NewFromEnv() *Client {
	return &Client{
		Client: aws.NewFromEnv(),
	}
}

type Bucket struct {
	Name         string    `xml:"Name"`
	CreationDate time.Time `xml:"CreationDate"`
}

type ListAllMyBucketsResult struct {
	XMLName          xml.Name `xml:"ListAllMyBucketsResult"`
	OwnerID          string   `xml:"Owner>ID"`
	OwnerDisplayName string   `xml:"Owner>DisplayName"`

	Buckets []*Bucket `xml:"Buckets>Bucket"`
}

func (client *Client) EndpointHost() string {
	if client.CustomEndpointHost != "" {
		return client.CustomEndpointHost
	}
	return DEFAULT_ENDPOINT_HOST
}

func (client *Client) Endpoint() string {
	if client.UseSsl {
		return "https://" + client.EndpointHost()
	} else {
		return "http://" + client.EndpointHost()
	}
}

type PutOptions struct {
	ContentType          string
	ContentLength        int
	ContentEncoding      string
	AmzAcl               string
	ServerSideEncryption bool
	MetaHeader           http.Header
}

func NewPublicPut() *PutOptions {
	return &PutOptions{
		AmzAcl: AMZ_ACL_PUBLIC,
	}
}

type Content struct {
	Key              string    `xml:"Key"`
	LastModified     time.Time `xml:"LastModified"`
	Etag             string    `xml:"ETag"`
	Size             int64     `xml:"Size"`
	StorageClass     string    `xml:"StorageClass"`
	OwnerID          string    `xml:"Owner>ID"`
	OwnerDisplayName string    `xml:"Owner>DisplayName"`
}

type ListBucketResult struct {
	XMLName     xml.Name `xml:"ListBucketResult"`
	Name        string   `xml:"Name"`
	Prefix      string   `xml:"Prefix"`
	Marker      string   `xml:"Marker"`
	MaxKeys     int      `xml:"MaxKeys"`
	IsTruncated bool     `xml:"IsTruncated"`

	Contents []*Content `xml:"Contents"`
}

type ApiError struct {
	Message      string
	Request      *http.Request
	Response     *http.Response
	ResponseBody []byte
}

func NewApiError(message string, req *http.Request, rsp *http.Response, body []byte) *ApiError {
	return &ApiError{
		Message:      message,
		Request:      req,
		Response:     rsp,
		ResponseBody: body,
	}
}

func (e ApiError) Error() string {
	return fmt.Sprintf("%s: status=%s", e.Message, e.Response.Status)
}

func (client *Client) Service() (r *ListAllMyBucketsResult, e error) {
	req, e := http.NewRequest("GET", client.Endpoint()+"/", nil)
	if e != nil {
		return r, e
	}
	rsp, body, e := client.signAndDoRequest("", req)
	if e != nil {
		return nil, e
	}
	r = &ListAllMyBucketsResult{}
	e = xml.Unmarshal(body, r)
	if e != nil {
		return nil, NewApiError("Unmarshalling ListAllMyBucketsResult", req, rsp, body)
	}
	return r, e
}

func (client *Client) Head(bucket, key string) (*http.Response, error) {
	return client.readRequest("HEAD", bucket, key)
}

func (client *Client) Get(bucket, key string) (*http.Response, error) {
	return client.readRequest("GET", bucket, key)
}

func (client *Client) readRequest(method, bucket, key string) (*http.Response, error) {
	theUrl := client.keyUrl(bucket, key)
	req, e := http.NewRequest(method, theUrl, nil)
	if e != nil {
		return nil, e
	}
	client.SignS3Request(req, bucket)
	return http.DefaultClient.Do(req)
}

func (client *Client) keyUrl(bucket, key string) string {
	if client.UseSsl {
		return "https://" + client.EndpointHost() + "/" + bucket + "/" + key
	}
	return "http://" + bucket + "." + client.EndpointHost() + "/" + key
}

func (client *Client) PutStream(bucket, key string, r io.Reader, options *PutOptions) error {
	if options == nil {
		options = &PutOptions{ContentType: DEFAULT_CONTENT_TYPE}
	}
	theUrl := client.keyUrl(bucket, key)
	req, e := http.NewRequest("PUT", theUrl, r)
	if e != nil {
		return e
	}

	req.Header = client.putRequestHeaders(bucket, key, options)

	buf := bytes.NewBuffer(make([]byte, 0, MinPartSize))
	_, e = io.CopyN(buf, r, MinPartSize)
	if e == io.EOF {
		// less than min multipart size => direct upload
		return client.Put(bucket, key, buf.Bytes(), options)
	} else if e != nil {
		return e
	}
	mr := io.MultiReader(buf, r)

	mo := &MultipartOptions{
		PartSize: 5 * 1024 * 1024,
		Callback: func(res *UploadPartResult) {
			if res.Error != nil {
				logger.Print("ERROR: " + e.Error())
			}
		},
		PutOptions: options,
	}
	_, e = client.PutMultipart(bucket, key, mr, mo)
	return e
}

func (client *Client) putRequestHeaders(bucket, key string, options *PutOptions) http.Header {
	headers := http.Header{}
	headers.Add("Host", bucket+"."+client.EndpointHost())

	if options.MetaHeader != nil {
		for k := range options.MetaHeader {
			headers.Add("x-amz-meta-"+k, options.MetaHeader.Get(k))
		}
	}

	contentType := options.ContentType
	if contentType == "" {
		contentType = DEFAULT_CONTENT_TYPE
	}
	headers.Add(HEADER_CONTENT_TYPE, contentType)

	if options.ContentEncoding != "" {
		headers.Add("Content-Encoding", options.ContentEncoding)
	}

	if options.AmzAcl != "" {
		headers.Add(HEADER_AMZ_ACL, options.AmzAcl)
	}

	if options.ServerSideEncryption {
		headers.Add(HEADER_SERVER_SIDE_ENCRUPTION, AES256)
	}
	return headers
}

func (client *Client) Delete(bucket, key string) error {
	req, e := http.NewRequest("DELETE", client.keyUrl(bucket, key), nil)
	if e != nil {
		return e
	}
	client.SignS3Request(req, bucket)

	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return e
	}
	defer rsp.Body.Close()
	return nil
}

func (client *Client) Put(bucket, key string, data []byte, options *PutOptions) error {
	if options == nil {
		options = &PutOptions{ContentType: DEFAULT_CONTENT_TYPE}
	}

	buf := bytes.NewBuffer(data)
	theUrl := client.keyUrl(bucket, key)
	req, e := http.NewRequest("PUT", theUrl, buf)
	if e != nil {
		return e
	}
	req.Header = client.putRequestHeaders(bucket, key, options)

	b64md5, e := contentMd5(string(data))
	if e != nil {
		return e
	}
	req.Header.Add(HEADER_CONTENT_MD5, b64md5)

	client.SignS3Request(req, bucket)
	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return e
	}
	if rsp.StatusCode != 200 {
		return fmt.Errorf("error uploading key: %s - %s", rsp.Status, string(b))
	}
	return nil
}

// stolen from goamz
var s3ParamsToSign = map[string]bool{
	"acl":                          true,
	"location":                     true,
	"logging":                      true,
	"notification":                 true,
	"partNumber":                   true,
	"policy":                       true,
	"requestPayment":               true,
	"torrent":                      true,
	"uploadId":                     true,
	"uploads":                      true,
	"versionId":                    true,
	"versioning":                   true,
	"versions":                     true,
	"response-content-type":        true,
	"response-content-language":    true,
	"response-expires":             true,
	"response-cache-control":       true,
	"response-content-disposition": true,
	"response-content-encoding":    true,
}

func (client *Client) SignS3Request(req *http.Request, bucket string) {
	t := time.Now().UTC()
	date := t.Format(http.TimeFormat)
	if client.Client.SecurityToken != "" {
		req.Header.Set("x-amz-security-token", client.Client.SecurityToken)
	}
	payloadParts := []string{
		req.Method,
		req.Header.Get(HEADER_CONTENT_MD5),
		req.Header.Get(HEADER_CONTENT_TYPE),
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
	path := req.URL.Path
	query := normalizeParams(req.URL)
	if query != "" {
		path += "?" + query
	}
	if !client.UseSsl && bucket != "" {
		path = "/" + bucket + path
	}
	payloadParts = append(payloadParts, path)
	payload := strings.Join(payloadParts, "\n")
	req.Header.Add(HEADER_DATE, date)
	req.Header.Add(HEADER_AUTHORIZATION, "AWS "+client.Key+":"+signPayload(payload, client.newSha1Hash(client.Secret)))
}

func normalizeParams(url *url.URL) string {
	params := []string{}
	for _, part := range strings.Split(url.RawQuery, "&") {
		parts := strings.SplitN(part, "=", 2)
		if _, ok := s3ParamsToSign[parts[0]]; ok {
			params = append(params, part)
		}
	}
	sort.Strings(params)
	if len(params) > 0 {
		return strings.Join(params, "&")
	}
	return ""
}

func (client *Client) newSha1Hash(secret string) hash.Hash {
	return hmac.New(sha1.New, []byte(client.Secret))
}

func signPayload(payload string, hash hash.Hash) string {
	hash.Write([]byte(payload))
	signature := make([]byte, b64.EncodedLen(hash.Size()))
	b64.Encode(signature, hash.Sum(nil))
	return string(signature)
}

func (client *Client) signAndDoRequest(bucket string, req *http.Request) (rsp *http.Response, body []byte, e error) {
	client.SignS3Request(req, bucket)
	rsp, e = http.DefaultClient.Do(req)
	if e != nil {
		return rsp, nil, e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	return rsp, b, e
}
