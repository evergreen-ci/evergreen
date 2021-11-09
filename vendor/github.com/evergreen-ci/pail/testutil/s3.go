package testutil

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
)

func CleanupS3Bucket(creds *credentials.Credentials, name, prefix, region string) error {
	svc, err := CreateS3Client(creds, region)
	if err != nil {
		return errors.Wrap(err, "clean up failed")
	}
	deleteObjectsInput := &s3.DeleteObjectsInput{
		Bucket: aws.String(name),
		Delete: &s3.Delete{},
	}
	listInput := &s3.ListObjectsInput{
		Bucket: aws.String(name),
		Prefix: aws.String(prefix),
	}
	var result *s3.ListObjectsOutput

	for {
		result, err = svc.ListObjects(listInput)
		if err != nil {
			return errors.Wrap(err, "clean up failed")
		}

		for _, object := range result.Contents {
			deleteObjectsInput.Delete.Objects = append(deleteObjectsInput.Delete.Objects, &s3.ObjectIdentifier{
				Key: object.Key,
			})
		}

		if deleteObjectsInput.Delete.Objects != nil {
			_, err = svc.DeleteObjects(deleteObjectsInput)
			if err != nil {
				return errors.Wrap(err, "failed to delete S3 bucket")
			}
			deleteObjectsInput.Delete = &s3.Delete{}
		}

		if *result.IsTruncated {
			listInput.Marker = result.Contents[len(result.Contents)-1].Key
		} else {
			break
		}
	}

	return nil
}

func CreateS3Client(creds *credentials.Credentials, region string) (*s3.S3, error) {
	sess, err := session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(region),
	})
	if err != nil {
		return nil, errors.Wrap(err, "problem connecting to AWS")
	}
	svc := s3.New(sess)
	return svc, nil
}
