package thirdparty

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	awsSDK "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awsS3 "github.com/aws/aws-sdk-go/service/s3"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/s3"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

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

const (
	s3ConnectTimeout = 2 * time.Minute
	s3ReadTimeout    = 10 * time.Minute
	s3WriteTimeout   = 10 * time.Minute
	newCode          = true
	region           = "us-east-1"
	// Minimum 5MB per chunk except for last part http://docs.aws.amazon.com/AmazonS3/latest/API/mpUploadComplete.html
	maxPartSize = 1024 * 1024 * 5
)

// For our S3 copy operations, S3 either returns an CopyObjectResult or
// a CopyObjectError body. In order to determine what kind of response
// was returned we read the body returned from the API call
type CopyObjectResult struct {
	XMLName      xml.Name `xml:"CopyObjectResult"`
	LastModified string   `xml:"LastModified"`
	ETag         string   `xml:"ETag"`
}

type CopyObjectError struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource"`
	RequestId string   `xml:"RequestId"`
	ErrMsg    string
}

func (e CopyObjectError) Error() string {
	return fmt.Sprintf("Code: %v\nMessage: %v\nResource: %v"+
		"\nRequestId: %v\nErrMsg: %v\n",
		e.Code, e.Message, e.Resource, e.RequestId, e.ErrMsg)
}

// NewS3Session returns a configures s3 session.
func NewS3Session(auth *aws.Auth, region aws.Region, client *http.Client) *s3.S3 {
	s3Session := s3.New(*auth, region, client)
	s3Session.ReadTimeout = s3ReadTimeout
	s3Session.WriteTimeout = s3WriteTimeout
	s3Session.ConnectTimeout = s3ConnectTimeout
	return s3Session
}

func S3CopyFile(awsAuth *aws.Auth, fromS3Bucket, fromS3Path, toS3Bucket, toS3Path, permissionACL string) error {
	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	destinationPath := fmt.Sprintf("http://%v.s3.amazonaws.com/%v",
		toS3Bucket, toS3Path)
	req, err := http.NewRequest("PUT", destinationPath, nil)
	if err != nil {
		return errors.Wrapf(err, "PUT request on %v failed", destinationPath)
	}
	req.Header.Add("x-amz-copy-source", fmt.Sprintf("/%v/%v", fromS3Bucket,
		fromS3Path))
	req.Header.Add("x-amz-date", time.Now().Format(time.RFC850))
	if permissionACL != "" {
		req.Header.Add("x-amz-acl", permissionACL)
	}
	signaturePath := fmt.Sprintf("/%v/%v", toS3Bucket, toS3Path)
	SignAWSRequest(*awsAuth, signaturePath, req)

	resp, err := client.Do(req)
	if resp == nil {
		return errors.Wrap(err, "Nil response received")
	}
	defer resp.Body.Close()

	// attempt to read the response body to check for success/error message
	respBody, respBodyErr := ioutil.ReadAll(resp.Body)
	if respBodyErr != nil {
		return errors.Errorf("Error reading s3 copy response body: %v", respBodyErr)
	}

	// Attempt to unmarshall the response body. If there's no errors, it means
	// that the S3 copy was successful. If there's an error, or a non-200
	// response code, it indicates a copy error
	copyObjectResult := CopyObjectResult{}
	xmlErr := xml.Unmarshal(respBody, &copyObjectResult)
	if xmlErr != nil || resp.StatusCode != http.StatusOK {
		var errMsg string
		if xmlErr == nil {
			errMsg = fmt.Sprintf("S3 returned status code: %d", resp.StatusCode)
		} else {
			errMsg = fmt.Sprintf("unmarshalling error: %v", xmlErr)
		}
		// an unmarshalling error or a non-200 status code indicates S3 returned
		// an error so we'll now attempt to unmarshall that error response
		copyObjectError := CopyObjectError{}
		xmlErr = xml.Unmarshal(respBody, &copyObjectError)
		if xmlErr != nil {
			// *This should seldom happen since a non-200 status code or a
			// copyObjectResult unmarshall error on a response from S3 should
			// contain a CopyObjectError. An error here indicates possible
			// backwards incompatible changes in the AWS API
			return errors.Errorf("Unrecognized S3 response: %v: %v", errMsg, xmlErr)
		}
		copyObjectError.ErrMsg = errMsg
		// if we were able to parse out an error response, then we can reliably
		// inform the user of the error
		return copyObjectError
	}
	return err
}

// PutS3File writes the specified file to an s3 bucket using the given permissions and content type.
// The details of where to put the file are included in the s3URL
func legacyPutS3File(pushAuth *aws.Auth, localFilePath, s3URL, contentType, permissionACL string) error {
	urlParsed, err := url.Parse(s3URL)
	if err != nil {
		return err
	}

	if urlParsed.Scheme != "s3" {
		return errors.Errorf("Don't know how to use URL with scheme %v", urlParsed.Scheme)
	}

	fi, err := os.Stat(localFilePath)
	if err != nil {
		return err
	}

	localFileReader, err := os.Open(localFilePath)
	if err != nil {
		return err
	}
	defer localFileReader.Close()

	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	session := NewS3Session(pushAuth, aws.USEast, client)
	bucket := session.Bucket(urlParsed.Host)
	// options for the header
	options := s3.Options{}
	return errors.Wrapf(bucket.PutReader(urlParsed.Path, localFileReader, fi.Size(), contentType, s3.ACL(permissionACL), options),
		"problem putting %s to bucket", localFilePath)
}

func PutS3File(pushAuth *aws.Auth, localFilePath, s3URL, contentType, permissionACL string) error {
	if !newCode {
		return legacyPutS3File(pushAuth, localFilePath, s3URL, contentType, permissionACL)
	}

	urlParsed, err := url.Parse(s3URL)
	if err != nil {
		return errors.Wrapf(err, "Error parsing URL: %s", s3URL)
	}

	if urlParsed.Scheme != "s3" {
		return errors.Errorf("Don't know how to use URL with scheme %v", urlParsed.Scheme)
	}

	config := &awsSDK.Config{
		Credentials: credentials.NewStaticCredentials(pushAuth.AccessKey, pushAuth.SecretKey, pushAuth.Token()),
		Region:      awsSDK.String(region),
	}
	session, err := session.NewSession(config)
	if err != nil {
		return errors.Wrap(err, "error creating new session")
	}
	svc := awsS3.New(session)
	bucket := &awsS3.CreateBucketInput{
		Bucket: &urlParsed.Host,
	}
	bucket = bucket.SetACL(permissionACL)

	file, err := os.Open(localFilePath)
	if err != nil {
		return errors.Wrapf(err, "Error opening file %s", localFilePath)
	}
	defer file.Close()

	// Get size of file to read into buffer
	fileInfo, err := file.Stat()
	if err != nil {
		return errors.Wrapf(err, "Error getting stats for file %s", localFilePath)
	}
	size := fileInfo.Size()
	buffer, err := ioutil.ReadAll(file)
	if err != nil {
		return errors.Wrapf(err, "Error reading bytes of file %s", localFilePath)
	}

	// Step 1: initiate multipart upload
	resp, err := createPart(svc, bucket, urlParsed.Path, contentType)
	if err != nil {
		return errors.Wrap(err, "Error initiating part")
	}

	// Step 2: upload each part
	completedParts := make([]*awsS3.CompletedPart, 0)
	partNum := int64(1)
	remaining := size
	var partLength int64
	// Keep uploading until there are no many bytes remaining
	for i := int64(0); remaining != 0; i += partLength {
		// Abide by 5MB per part limit
		if remaining < maxPartSize {
			partLength = remaining
		} else {
			partLength = maxPartSize
		}

		uploadedPart, err := uploadPart(svc, resp, buffer[i:i+partLength], partNum)
		if err != nil {
			//  If an error occurs, abort upload to not get charged by Amazon
			abortErr := abortMultipartUpload(svc, resp)
			if abortErr != nil {
				return errors.Wrapf(abortErr, "Error aborting multipart upload for %s", localFilePath)
			}
			return errors.Wrapf(err, "Error uploading part in S3 multipart upload for %s", localFilePath)
		}
		remaining -= partLength
		partNum++
		completedParts = append(completedParts, uploadedPart)
	}

	// Step 3: complete upload by assembling all uploaded parts
	return completeMultipartUpload(svc, resp, completedParts)
}

// createPart initiates a multipart upload
func createPart(svc *awsS3.S3, bucket *awsS3.CreateBucketInput, path, contentType string) (*awsS3.CreateMultipartUploadOutput, error) {
	creationInput := &awsS3.CreateMultipartUploadInput{
		ACL:         bucket.ACL,
		Bucket:      bucket.Bucket,
		Key:         awsSDK.String(path),
		ContentType: awsSDK.String(contentType),
	}

	result, err := svc.CreateMultipartUpload(creationInput)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating part for multipart upload")
	}
	return result, nil
}

// uploadPart uploads a part by identifying a partNum and its position within the
// object being created
func uploadPart(svc *awsS3.S3, resp *awsS3.CreateMultipartUploadOutput,
	fileBytes []byte, partNum int64) (*awsS3.CompletedPart, error) {
	uploadInput := &awsS3.UploadPartInput{
		Body:          bytes.NewReader(fileBytes),
		Bucket:        awsSDK.String(*resp.Bucket),
		Key:           awsSDK.String(*resp.Key),
		PartNumber:    awsSDK.Int64(partNum),
		UploadId:      awsSDK.String(*resp.UploadId),
		ContentLength: awsSDK.Int64(int64(len(fileBytes))),
	}
	uploadResult, err := svc.UploadPart(uploadInput)
	if err != nil {
		return nil, errors.Wrap(err, "Error uploading parts to multipart upload")
	}
	uploadedPart := &awsS3.CompletedPart{
		ETag:       awsSDK.String(*uploadResult.ETag),
		PartNumber: awsSDK.Int64(partNum),
	}
	return uploadedPart, nil
}

// completeMultipartUpload completes a multipart upload by assembling previously
// uploaded parts in order of ascending part number
func completeMultipartUpload(svc *awsS3.S3, resp *awsS3.CreateMultipartUploadOutput,
	completedParts []*awsS3.CompletedPart) error {
	completeInput := &awsS3.CompleteMultipartUploadInput{
		Bucket: awsSDK.String(*resp.Bucket),
		Key:    awsSDK.String(*resp.Key),
		MultipartUpload: &awsS3.CompletedMultipartUpload{
			Parts: completedParts,
		},
		UploadId: awsSDK.String(*resp.UploadId),
	}
	_, err := svc.CompleteMultipartUpload(completeInput)
	if err != nil {
		return errors.Wrap(err, "Error completing multipart upload")
	}
	return nil
}

// abortMultipartUpload aborts a multipart upload to stop getting charged for
// storage of the uploaded parts
func abortMultipartUpload(svc *awsS3.S3, resp *awsS3.CreateMultipartUploadOutput) error {
	abortInput := &awsS3.AbortMultipartUploadInput{
		Bucket:   awsSDK.String(*resp.Bucket),
		Key:      awsSDK.String(*resp.Key),
		UploadId: awsSDK.String(*resp.UploadId),
	}
	_, err := svc.AbortMultipartUpload(abortInput)
	if err != nil {
		return errors.Wrap(err, "Error aborting multipart upload")
	}
	return nil
}

func legacyGetS3File(auth *aws.Auth, s3URL string) (io.ReadCloser, error) {
	urlParsed, err := url.Parse(s3URL)
	if err != nil {
		return nil, err
	}
	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	session := NewS3Session(auth, aws.USEast, client)

	bucket := session.Bucket(urlParsed.Host)
	return bucket.GetReader(urlParsed.Path)
}

func GetS3File(auth *aws.Auth, s3URL string) (io.ReadCloser, error) {
	if !newCode {
		return legacyGetS3File(auth, s3URL)
	}

	urlParsed, err := url.Parse(s3URL)
	if err != nil {
		return nil, errors.Wrapf(err, "Error parsing URL: %s", s3URL)
	}

	config := &awsSDK.Config{
		Credentials: credentials.NewStaticCredentials(auth.AccessKey, auth.SecretKey, auth.Token()),
		Region:      awsSDK.String(region),
	}
	session, err := session.NewSession(config)
	if err != nil {
		return nil, errors.Wrap(err, "error creating new session")
	}

	svc := awsS3.New(session)
	bucket := awsS3.CreateBucketInput{
		Bucket: &urlParsed.Host,
	}

	input := &awsS3.GetObjectInput{
		Bucket: bucket.Bucket,
		Key:    awsSDK.String(urlParsed.Path),
	}
	rc, err := svc.GetObject(input)
	if err != nil {
		return nil, errors.Wrap(err, "Error getting s3 file")
	}
	return rc.Body, nil
}

//Taken from https://github.com/mitchellh/goamz/blob/master/s3/sign.go
//Modified to access the headers/params on an HTTP req directly.
func SignAWSRequest(auth aws.Auth, canonicalPath string, req *http.Request) {
	method := req.Method
	headers := req.Header
	params := req.URL.Query()

	var md5, ctype, date, xamz string
	var xamzDate bool
	var sarray []string
	var err error
	for k, v := range headers {
		k = strings.ToLower(k)
		switch k {
		case "content-md5":
			md5 = v[0]
		case "content-type":
			ctype = v[0]
		case "date":
			if !xamzDate {
				date = v[0]
			}
		default:
			if strings.HasPrefix(k, "x-amz-") {
				vall := strings.Join(v, ",")
				sarray = append(sarray, k+":"+vall)
				if k == "x-amz-date" {
					xamzDate = true
					date = ""
				}
			}
		}
	}
	if len(sarray) > 0 {
		sort.StringSlice(sarray).Sort()
		xamz = strings.Join(sarray, "\n") + "\n"
	}

	expires := false
	if v, ok := params["Expires"]; ok {
		// Query string request authentication alternative.
		expires = true
		date = v[0]
		params["AWSAccessKeyId"] = []string{auth.AccessKey}
	}

	sarray = sarray[0:0]
	for k, v := range params {
		if s3ParamsToSign[k] {
			for _, vi := range v {
				if vi == "" {
					sarray = append(sarray, k)
				} else {
					// "When signing you do not encode these values."
					sarray = append(sarray, k+"="+vi)
				}
			}
		}
	}
	if len(sarray) > 0 {
		sort.StringSlice(sarray).Sort()
		canonicalPath = canonicalPath + "?" + strings.Join(sarray, "&")
	}

	payload := method + "\n" + md5 + "\n" + ctype + "\n" + date + "\n" + xamz + canonicalPath
	hash := hmac.New(sha1.New, []byte(auth.SecretKey))
	_, err = hash.Write([]byte(payload))
	grip.Debug(err)

	signature := make([]byte, base64.StdEncoding.EncodedLen(hash.Size()))
	base64.StdEncoding.Encode(signature, hash.Sum(nil))

	if expires {
		params["Signature"] = []string{string(signature)}
	} else {
		headers["Authorization"] = []string{"AWS " + auth.AccessKey + ":" + string(signature)}
	}
}
