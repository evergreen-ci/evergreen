package s3

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
)

type InitiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadId string   `xml:"UploadId"`
}

type CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

type Part struct {
	ETag       string
	PartNumber int
}

type CompleteMultipartUpload struct {
	XMLName xml.Name `xml:"CompleteMultipartUpload"`
	Parts   []*Part  `xml:"Part"`
}

type MultipartOptions struct {
	*PutOptions
	PartSize int
	Callback func(*UploadPartResult)
}

const MinPartSize = 5 * 1024 * 1024

type UploadPartResult struct {
	CurrentSize int
	Part        *Part
	Error       error
}

func (client *Client) PutMultipart(bucket, key string, f io.Reader, opts *MultipartOptions) (res *CompleteMultipartUploadResult, e error) {
	if opts == nil {
		opts = &MultipartOptions{
			PartSize: MinPartSize,
		}
	}
	if opts.PartSize == 0 {
		opts.PartSize = MinPartSize
	}

	if opts.PartSize < MinPartSize {
		return nil, fmt.Errorf("part size must be at least %d but was %d", MinPartSize, opts.PartSize)
	}

	result, e := client.InitiateMultipartUpload(bucket, key, opts.PutOptions)
	if e != nil {
		return nil, e
	}
	partId := 1
	parts := []*Part{}
	currentSize := 0
	for {
		buf := bytes.NewBuffer(make([]byte, 0, opts.PartSize))
		i, e := io.CopyN(buf, f, int64(opts.PartSize))
		if e != nil && e != io.EOF {
			return nil, e
		}
		if i > 0 {
			part, e := client.UploadPart(bucket, key, buf.Bytes(), partId, result.UploadId)
			if opts.Callback != nil {
				opts.Callback(&UploadPartResult{Part: part, Error: e, CurrentSize: currentSize})
			}
			if e != nil {
				return nil, e
			}
			currentSize += buf.Len()
			parts = append(parts, part)
			partId++
		}
		if e == io.EOF {
			break
		}
	}
	return client.CompleteMultipartUpload(bucket, key, result.UploadId, parts)
}

func (client *Client) InitiateMultipartUpload(bucket, key string, opts *PutOptions) (result *InitiateMultipartUploadResult, e error) {
	if opts == nil {
		opts = &PutOptions{}
	}
	theUrl := client.keyUrl(bucket, key) + "?uploads"
	req, e := http.NewRequest("POST", theUrl, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Add("Host", bucket+"."+client.EndpointHost())
	if opts.ServerSideEncryption {
		req.Header.Add(HEADER_SERVER_SIDE_ENCRUPTION, AES256)
	}
	client.SignS3Request(req, bucket)
	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return nil, e
	}
	result = &InitiateMultipartUploadResult{}
	e = xml.Unmarshal(b, result)
	if e != nil {
		return nil, fmt.Errorf("ERROR: %s %s", e, string(b))
	}
	return result, e
}

func (client *Client) UploadPart(bucket, key string, data []byte, partId int, uploadId string) (part *Part, e error) {
	theUrl := client.keyUrl(bucket, key) + fmt.Sprintf("?partNumber=%d&uploadId=%s", partId, uploadId)
	req, e := http.NewRequest("PUT", theUrl, bytes.NewBuffer(data))
	if e != nil {
		return nil, e
	}
	req.Header.Add("Content-Length", strconv.Itoa(len(data)))
	client.SignS3Request(req, bucket)
	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()
	if rsp.Status[0] != '2' {
		b, _ := ioutil.ReadAll(rsp.Body)
		return nil, fmt.Errorf("expected status 2xx, got %s (%s)", rsp.Status, string(b))
	}
	return &Part{ETag: rsp.Header.Get("ETag"), PartNumber: partId}, nil
}

func (client *Client) CompleteMultipartUpload(bucket, key, uploadId string, parts []*Part) (result *CompleteMultipartUploadResult, e error) {
	theUrl := client.keyUrl(bucket, key) + fmt.Sprintf("?uploadId=%s", uploadId)
	payload := &CompleteMultipartUpload{Parts: parts}
	buf := &bytes.Buffer{}
	e = xml.NewEncoder(buf).Encode(payload)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequest("POST", theUrl, buf)
	if e != nil {
		return nil, e
	}
	req.Header.Add("Host", bucket+"."+client.EndpointHost())
	client.SignS3Request(req, bucket)
	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()
	if e != nil {
		return nil, e
	}
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return nil, e
	}
	if rsp.Status[0] != '2' {
		return nil, fmt.Errorf("expected status 2xx but got %s (%s)", rsp.Status, string(b))
	}
	result = &CompleteMultipartUploadResult{}
	e = xml.Unmarshal(b, result)
	if e != nil {
		return nil, e
	}
	return result, e
}
