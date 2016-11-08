package aws

import (
	"net/http"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSignAwsRequestV2(t *testing.T) {
	Convey("SignAwsRequestV2", t, func() {
		client := Client{
			Key:    "AKIAIOSFODNN7EXAMPLE",
			Secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		}
		time.Now()
		refTime := time.Date(2011, time.October, 3, 15, 19, 30, 0, time.UTC)
		req, e := http.NewRequest("GET", "https://elasticmapreduce.amazonaws.com/?Action=DescribeJobFlows&Version=2009-03-31", nil)
		So(e, ShouldBeNil)
		payload, _ := client.v2PayloadAndQuery(req, refTime)
		So(payload, ShouldContainSubstring, "AWSAccessKeyId=AKIAIOSFODNN7EXAMPLE&Action=DescribeJobFlows&SignatureMethod=HmacSHA256&SignatureVersion=2&Timestamp=2011-10-03T15%3A19%3A30&Version=2009-03-31")
		client.SignAwsRequestV2(req, refTime)
		raw := req.URL.RawQuery

		So(raw, ShouldContainSubstring, "SignatureMethod=HmacSHA256")
		So(raw, ShouldContainSubstring, "AWSAccessKeyId=AKIAIOSFODNN7EXAMPLE")
		So(raw, ShouldContainSubstring, "SignatureVersion=2")
		So(raw, ShouldContainSubstring, "Timestamp=2011-10-03T15%3A19%3A30")
		So(raw, ShouldContainSubstring, "Version=2009-03-31")
		So(raw, ShouldContainSubstring, "Signature=i91nKc4PWAt0JJIdXwz9HxZCJDdiy6cf%2FMj6vPxyYIs%3D")
	})
}

func TestSignS3Request(t *testing.T) {
	Convey("SignS3Request", t, func() {
		client := Client{
			Key:    "44CF9590006BF252F707",
			Secret: "OtxrzxIsfpFjA7SwPzILwy8Bw21TLhquhboDYROV",
		}
		req, e := http.NewRequest("PUT", "/quotes/nelson", nil)
		So(e, ShouldBeNil)
		req.Header.Add("Content-Md5", "c8fdb181845a4ca6b8fec737b3581d76")
		req.Header.Add("Content-Type", "text/html")
		req.Header.Add("Date", "Thu, 17 Nov 2005 18:49:58 GMT")
		req.Header.Add("X-Amz-Meta-Author", "foo@bar.com")
		req.Header.Add("X-Amz-Magic", "abracadabra")

		payload := s3Payload(req)
		So(payload, ShouldStartWith, "PUT\nc8fdb181845a4ca6b8fec737b3581d76\ntext/html\nThu, 17 Nov 2005 18:49:58 GMT\nx-amz-magic:abracadabra\nx-amz-meta-author:foo@bar.com\n/quotes/nelson")

		client.SignS3Request(req)
		So(req.Header.Get("Authorization"), ShouldEqual, "AWS 44CF9590006BF252F707:jZNOcbfWmD/A/f3hSvVzXZjM2HU=")
	})
}
