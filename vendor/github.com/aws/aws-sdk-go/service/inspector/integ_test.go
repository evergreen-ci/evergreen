// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// +build go1.10,integration

package inspector_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/awstesting/integration"
	"github.com/aws/aws-sdk-go/service/inspector"
)

var _ aws.Config
var _ awserr.Error
var _ request.Request

func TestInteg_00_ListAssessmentTemplates(t *testing.T) {
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()

	sess := integration.SessionWithDefaultRegion("us-west-2")
	svc := inspector.New(sess)
	params := &inspector.ListAssessmentTemplatesInput{}
	_, err := svc.ListAssessmentTemplatesWithContext(ctx, params)
	if err != nil {
		t.Errorf("expect no error, got %v", err)
	}
}
func TestInteg_01_ListTagsForResource(t *testing.T) {
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()

	sess := integration.SessionWithDefaultRegion("us-west-2")
	svc := inspector.New(sess)
	params := &inspector.ListTagsForResourceInput{
		ResourceArn: aws.String("fake-arn"),
	}
	_, err := svc.ListTagsForResourceWithContext(ctx, params)
	if err == nil {
		t.Fatalf("expect request to fail")
	}
	aerr, ok := err.(awserr.RequestFailure)
	if !ok {
		t.Fatalf("expect awserr, was %T", err)
	}
	if len(aerr.Code()) == 0 {
		t.Errorf("expect non-empty error code")
	}
	if v := aerr.Code(); v == request.ErrCodeSerialization {
		t.Errorf("expect API error code got serialization failure")
	}
}
