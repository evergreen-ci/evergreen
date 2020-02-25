package thirdparty

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

//PresignExpireTime sets the amount of time the link is live before expiring
const PresignExpireTime = 24 * time.Hour

//RequestParams holds all the parameters needed to sign a url
type RequestParams struct {
	Bucket    string `json:"bucket"`
	FileKey   string `json:"fileKey"`
	AwsKey    string `json:"awsKey"`
	AwsSecret string `json:"awsSecret"`
}

//PreSign returns a presigned url that expires in 24 hours
func PreSign(r RequestParams) (string, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(endpoints.UsEast1RegionID),
		Credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     r.AwsKey,
			SecretAccessKey: r.AwsSecret,
		}),
	})
	if err != nil {
		return "", err
	}
	svc := s3.New(sess)

	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(r.Bucket),
		Key:    aws.String(r.FileKey),
	})

	urlStr, err := req.Presign(PresignExpireTime)
	return urlStr, err
}
