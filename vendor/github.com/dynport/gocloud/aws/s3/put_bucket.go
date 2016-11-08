package s3

import (
	"net/http"
)

// http://docs.aws.amazon.com/AmazonS3/latest/API/RESTBucketPUT.html
func (client *Client) PutBucket(name string) error {
	req, e := http.NewRequest("PUT", "http://"+client.EndpointHost()+"/", nil)
	if e != nil {
		return e
	}
	req.Header.Add("Host", name+"."+client.EndpointHost())
	req.Header.Add("Content-Length", "0")
	rsp, b, e := client.signAndDoRequest("", req)
	if e != nil {
		return e
	}
	if rsp.Status[0] != '2' {
		return NewApiError("Unable to create bucket "+name, req, rsp, b)

	}
	return nil
}
