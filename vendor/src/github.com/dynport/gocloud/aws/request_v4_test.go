package aws

import (
	"net/url"
	"strings"
	"testing"
	"time"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	defaultUrl  = "http://iam.amazonaws.com/"
	defaultTime = time.Date(2011, 9, 9, 23, 36, 0, 0, time.UTC)
	secret      = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	key         = "AKIDEXAMPLE"
)

func newAwsRequest(theUrl *url.URL) *RequestV4 {
	r := &RequestV4{
		Method:  "POST",
		URL:     theUrl,
		Payload: []byte("Action=ListUsers&Version=2010-05-08"),
		Region:  "us-east-1",
		Service: "iam",
		Time:    defaultTime,
		Key:     key,
		Secret:  secret,
	}
	r.SetHeader("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	return r
}

func TestValidatedHeaders(t *testing.T) {
	theUrl, e := url.Parse(defaultUrl)
	if e != nil {
		t.Fatal(e)
	}
	req := newAwsRequest(theUrl)
	req.SetHeader("x-amz-date", "20110909T233600Z")
	req.SetHeader("host", "iam.amazonaws.com")

	Convey("TestValidatedHeaders", t, func() {
		So(req, ShouldNotBeNil)
		_, signed, e := req.canonicalHeadersAndValues()
		So(e, ShouldBeNil)
		So(string(signed), ShouldNotContainSubstring, "host;host;x-amz-date;x-amz-date")
	})
}

func TestAwsRequest(t *testing.T) {
	Convey("Aws Request", t, func() {
		theUrl, e := url.Parse(defaultUrl)
		if e != nil {
			t.Fatal(e)
		}

		Convey("without payload", func() {
			awsRequest := newAwsRequest(theUrl)
			awsRequest.Payload = nil
			_, e := awsRequest.Request()
			So(e, ShouldBeNil)

		})
		Convey("without a date and host set set", func() {
			awsRequest := newAwsRequest(theUrl)
			req, e := awsRequest.Request()
			if e != nil {
				t.Fatal(e)
			}

			So(req.Header.Get("x-amz-date"), ShouldEqual, "20110909T233600Z")
			So(req.Header.Get("host"), ShouldEqual, "iam.amazonaws.com")
			So(1, ShouldEqual, 1)
		})
		// http://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
		awsRequest := newAwsRequest(theUrl)
		awsRequest.SetHeader("Content-type", "application/x-www-form-urlencoded; charset=utf-8")
		awsRequest.SetHeader("x-amz-date", "20110909T233600Z")
		awsRequest.SetHeader("host", "iam.amazonaws.com")

		So(awsRequest.Method, ShouldEqual, "POST")
		So(awsRequest.canonicalURI(), ShouldEqual, "/")
		So(awsRequest.canonicalQueryString(), ShouldEqual, "")
		ch, e := awsRequest.canonicalHeaders()
		So(e, ShouldBeNil)
		So(ch, ShouldEqual, "content-type:application/x-www-form-urlencoded; charset=utf-8\nhost:iam.amazonaws.com\nx-amz-date:20110909T233600Z\n")

		sh, e := awsRequest.signedHeaders()
		So(e, ShouldBeNil)
		So(sh, ShouldEqual, "content-type;host;x-amz-date")
		So(awsRequest.hashedPayload(), ShouldEqual, "b6359072c78d70ebee1e81adcbab4f01bf2c23245fa365ef83fe8f1f955085e2")

		hcr, e := awsRequest.hashedCanonicalRequest()
		So(e, ShouldBeNil)
		So(hcr, ShouldEqual, "3511de7e95d28ecd39e9513b642aee07e54f4941150d8df8bf94b328ef7e55e2")

		sts, e := awsRequest.stringToSign()
		So(e, ShouldBeNil)
		So(sts, ShouldContainSubstring, "AWS4-HMAC-SHA256\n")
		So(sts, ShouldContainSubstring, "20110909T233600Z\n")
		So(sts, ShouldContainSubstring, "20110909/us-east-1/iam/aws4_request\n")
		So(sts, ShouldContainSubstring, "3511de7e95d28ecd39e9513b642aee07e54f4941150d8df8bf94b328ef7e55e2")

		sig, e := awsRequest.signature()
		So(e, ShouldBeNil)
		So(sig, ShouldEqual, "ced6826de92d2bdeed8f846f0bf508e8559e98e4b0199114b84c54174deb456c")

		auth, e := awsRequest.authorization()
		So(e, ShouldBeNil)
		So(auth, ShouldNotEqual, "")
		So(auth, ShouldContainSubstring, "Signature=ced6826de92d2bdeed8f846f0bf508e8559e98e4b0199114b84c54174deb456c")
		So(auth, ShouldContainSubstring, "Credential=AKIDEXAMPLE/20110909/us-east-1/iam/aws4_request, ")
		So(auth, ShouldContainSubstring, "SignedHeaders=content-type;host;x-amz-date, ")

		v4Req, e := awsRequest.Request()
		if e != nil {
			t.Fatal(e)
		}
		So(v4Req, ShouldNotBeNil)

		So(v4Req.Method, ShouldEqual, "POST")
		So(v4Req.URL.RequestURI(), ShouldEqual, "/")
		So(v4Req.Header.Get("content-type"), ShouldEqual, "application/x-www-form-urlencoded; charset=utf-8")
		So(v4Req.Header.Get("Authorization"), ShouldStartWith, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20110909/us-east-1/iam/aws4_request,")

		Convey("Canonical Headers", func() {
			awsRequest := &RequestV4{Service: "iam", Region: "us-east-1", Key: key, Secret: secret}
			awsRequest.SetHeader("host", "iam.amazonaws.com")
			awsRequest.SetHeader("Content-type", "application/x-www-form-urlencoded; charset=utf-8")
			awsRequest.SetHeader("My-header1", "    a   b   c ")
			awsRequest.SetHeader("x-amz-date", "20120228T030031Z")
			awsRequest.SetHeader("My-Header2", `    "a   b   c"`)
			headers := []string{
				"content-type:application/x-www-form-urlencoded; charset=utf-8",
				"host:iam.amazonaws.com",
				"my-header1:a b c",
				`my-header2:"a   b   c"`,
				"x-amz-date:20120228T030031Z",
			}
			c, e := awsRequest.canonicalHeaders()
			So(e, ShouldBeNil)
			So(c, ShouldContainSubstring, `my-header2:"a   b   c"`+"\n")
			So(c, ShouldContainSubstring, "my-header1:a b c\n")
			h := strings.Join(headers, "\n") + "\n"
			So(c, ShouldEqual, h)
		})
	})
}
