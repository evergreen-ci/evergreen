package thirdparty

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/mongodb/grip"
)

//PresignExpireTime sets the amount of time the link is live before expiring
const PresignExpireTime = 24 * time.Hour

//PreSign returns a presigned url that expires in 24 hours
func PreSign(bucket string, filekey string, awsKey string, awsSecret string) (string, error ){
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(endpoints.UsEast1RegionID),
		Credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     awsKey,
			SecretAccessKey: awsSecret,
		}),
	})
	if err != nil {
		return "", err
	}
	svc := s3.New(sess)

	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(filekey),
	})

	urlStr, err := req.Presign(PresignExpireTime)
	return urlStr, err 
}
