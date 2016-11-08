package dynamodb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/dynport/gocloud/aws"
)

const targetPrefix = "DynamoDB_20120810"

func loadAction(client *aws.Client, action string, payload interface{}, rsp interface{}) error {
	u, e := url.Parse("http://dynamodb." + client.Region + ".amazonaws.com/")
	if e != nil {
		return e
	}
	b, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	debugger.Printf("PAYLOAD: %s", string(b))
	req := &aws.RequestV4{
		Service: "dynamodb",
		Region:  client.Region,
		Method:  "POST",
		URL:     u,
		Time:    time.Now(),
		Payload: b,
		Key:     client.Key,
		Secret:  client.Secret,
	}
	req.SetHeader("X-Amz-Target", targetPrefix+"."+action)
	req.SetHeader("Content-Type", "application/x-amz-json-1.0")

	r, e := req.Request()
	if e != nil {
		return e
	}

	started := time.Now()
	httpResponse, e := http.DefaultClient.Do(r)
	if e != nil {
		return e
	}
	defer httpResponse.Body.Close()
	debugger.Printf("RESPONSE: status=%s time=%.6f", httpResponse.Status, time.Since(started).Seconds())
	buf := &bytes.Buffer{}
	reader := io.TeeReader(httpResponse.Body, buf)
	e = json.NewDecoder(reader).Decode(rsp)
	debugger.Print("BODY: " + buf.String())
	if e != nil {
		return e
	} else if httpResponse.Status[0] != '2' {
		er := &ApiError{}
		e = json.Unmarshal(buf.Bytes(), er)
		if e != nil {
			return fmt.Errorf("status=%s error=%q", httpResponse.Status, e.Error())
		}
		return fmt.Errorf("status=%s type=%q error=%q", httpResponse.Status, er.Type, er.Message)
	}
	return nil
}

type ApiError struct {
	Type    string `json:"__type"`
	Message string `json:"message"`
}
