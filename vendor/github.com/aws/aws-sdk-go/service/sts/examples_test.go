// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package sts_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
)

var _ time.Duration
var _ strings.Reader
var _ aws.Config

func parseTime(layout, value string) *time.Time {
	t, err := time.Parse(layout, value)
	if err != nil {
		panic(err)
	}
	return &t
}

// To assume a role
//

func ExampleSTS_AssumeRole_shared00() {
	svc := sts.New(session.New())
	input := &sts.AssumeRoleInput{
		ExternalId:      aws.String("123ABC"),
		Policy:          aws.String("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Sid\":\"Stmt1\",\"Effect\":\"Allow\",\"Action\":\"s3:ListAllMyBuckets\",\"Resource\":\"*\"}]}"),
		RoleArn:         aws.String("arn:aws:iam::123456789012:role/demo"),
		RoleSessionName: aws.String("testAssumeRoleSession"),
		Tags: []*sts.Tag{
			{
				Key:   aws.String("Project"),
				Value: aws.String("Unicorn"),
			},
			{
				Key:   aws.String("Team"),
				Value: aws.String("Automation"),
			},
			{
				Key:   aws.String("Cost-Center"),
				Value: aws.String("12345"),
			},
		},
		TransitiveTagKeys: []*string{
			aws.String("Project"),
			aws.String("Cost-Center"),
		},
	}

	result, err := svc.AssumeRole(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case sts.ErrCodeMalformedPolicyDocumentException:
				fmt.Println(sts.ErrCodeMalformedPolicyDocumentException, aerr.Error())
			case sts.ErrCodePackedPolicyTooLargeException:
				fmt.Println(sts.ErrCodePackedPolicyTooLargeException, aerr.Error())
			case sts.ErrCodeRegionDisabledException:
				fmt.Println(sts.ErrCodeRegionDisabledException, aerr.Error())
			case sts.ErrCodeExpiredTokenException:
				fmt.Println(sts.ErrCodeExpiredTokenException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)
}

// To assume a role using a SAML assertion
//

func ExampleSTS_AssumeRoleWithSAML_shared00() {
	svc := sts.New(session.New())
	input := &sts.AssumeRoleWithSAMLInput{
		DurationSeconds: aws.Int64(3600),
		PrincipalArn:    aws.String("arn:aws:iam::123456789012:saml-provider/SAML-test"),
		RoleArn:         aws.String("arn:aws:iam::123456789012:role/TestSaml"),
		SAMLAssertion:   aws.String("VERYLONGENCODEDASSERTIONEXAMPLExzYW1sOkF1ZGllbmNlPmJsYW5rPC9zYW1sOkF1ZGllbmNlPjwvc2FtbDpBdWRpZW5jZVJlc3RyaWN0aW9uPjwvc2FtbDpDb25kaXRpb25zPjxzYW1sOlN1YmplY3Q+PHNhbWw6TmFtZUlEIEZvcm1hdD0idXJuOm9hc2lzOm5hbWVzOnRjOlNBTUw6Mi4wOm5hbWVpZC1mb3JtYXQ6dHJhbnNpZW50Ij5TYW1sRXhhbXBsZTwvc2FtbDpOYW1lSUQ+PHNhbWw6U3ViamVjdENvbmZpcm1hdGlvbiBNZXRob2Q9InVybjpvYXNpczpuYW1lczp0YzpTQU1MOjIuMDpjbTpiZWFyZXIiPjxzYW1sOlN1YmplY3RDb25maXJtYXRpb25EYXRhIE5vdE9uT3JBZnRlcj0iMjAxOS0xMS0wMVQyMDoyNTowNS4xNDVaIiBSZWNpcGllbnQ9Imh0dHBzOi8vc2lnbmluLmF3cy5hbWF6b24uY29tL3NhbWwiLz48L3NhbWw6U3ViamVjdENvbmZpcm1hdGlvbj48L3NhbWw6U3ViamVjdD48c2FtbDpBdXRoblN0YXRlbWVudCBBdXRoPD94bWwgdmpSZXNwb25zZT4="),
	}

	result, err := svc.AssumeRoleWithSAML(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case sts.ErrCodeMalformedPolicyDocumentException:
				fmt.Println(sts.ErrCodeMalformedPolicyDocumentException, aerr.Error())
			case sts.ErrCodePackedPolicyTooLargeException:
				fmt.Println(sts.ErrCodePackedPolicyTooLargeException, aerr.Error())
			case sts.ErrCodeIDPRejectedClaimException:
				fmt.Println(sts.ErrCodeIDPRejectedClaimException, aerr.Error())
			case sts.ErrCodeInvalidIdentityTokenException:
				fmt.Println(sts.ErrCodeInvalidIdentityTokenException, aerr.Error())
			case sts.ErrCodeExpiredTokenException:
				fmt.Println(sts.ErrCodeExpiredTokenException, aerr.Error())
			case sts.ErrCodeRegionDisabledException:
				fmt.Println(sts.ErrCodeRegionDisabledException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)
}

// To assume a role as an OpenID Connect-federated user
//

func ExampleSTS_AssumeRoleWithWebIdentity_shared00() {
	svc := sts.New(session.New())
	input := &sts.AssumeRoleWithWebIdentityInput{
		DurationSeconds:  aws.Int64(3600),
		Policy:           aws.String("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Sid\":\"Stmt1\",\"Effect\":\"Allow\",\"Action\":\"s3:ListAllMyBuckets\",\"Resource\":\"*\"}]}"),
		ProviderId:       aws.String("www.amazon.com"),
		RoleArn:          aws.String("arn:aws:iam::123456789012:role/FederatedWebIdentityRole"),
		RoleSessionName:  aws.String("app1"),
		WebIdentityToken: aws.String("Atza%7CIQEBLjAsAhRFiXuWpUXuRvQ9PZL3GMFcYevydwIUFAHZwXZXXXXXXXXJnrulxKDHwy87oGKPznh0D6bEQZTSCzyoCtL_8S07pLpr0zMbn6w1lfVZKNTBdDansFBmtGnIsIapjI6xKR02Yc_2bQ8LZbUXSGm6Ry6_BG7PrtLZtj_dfCTj92xNGed-CrKqjG7nPBjNIL016GGvuS5gSvPRUxWES3VYfm1wl7WTI7jn-Pcb6M-buCgHhFOzTQxod27L9CqnOLio7N3gZAGpsp6n1-AJBOCJckcyXe2c6uD0srOJeZlKUm2eTDVMf8IehDVI0r1QOnTV6KzzAI3OY87Vd_cVMQ"),
	}

	result, err := svc.AssumeRoleWithWebIdentity(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case sts.ErrCodeMalformedPolicyDocumentException:
				fmt.Println(sts.ErrCodeMalformedPolicyDocumentException, aerr.Error())
			case sts.ErrCodePackedPolicyTooLargeException:
				fmt.Println(sts.ErrCodePackedPolicyTooLargeException, aerr.Error())
			case sts.ErrCodeIDPRejectedClaimException:
				fmt.Println(sts.ErrCodeIDPRejectedClaimException, aerr.Error())
			case sts.ErrCodeIDPCommunicationErrorException:
				fmt.Println(sts.ErrCodeIDPCommunicationErrorException, aerr.Error())
			case sts.ErrCodeInvalidIdentityTokenException:
				fmt.Println(sts.ErrCodeInvalidIdentityTokenException, aerr.Error())
			case sts.ErrCodeExpiredTokenException:
				fmt.Println(sts.ErrCodeExpiredTokenException, aerr.Error())
			case sts.ErrCodeRegionDisabledException:
				fmt.Println(sts.ErrCodeRegionDisabledException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)
}

// To decode information about an authorization status of a request
//

func ExampleSTS_DecodeAuthorizationMessage_shared00() {
	svc := sts.New(session.New())
	input := &sts.DecodeAuthorizationMessageInput{
		EncodedMessage: aws.String("<encoded-message>"),
	}

	result, err := svc.DecodeAuthorizationMessage(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case sts.ErrCodeInvalidAuthorizationMessageException:
				fmt.Println(sts.ErrCodeInvalidAuthorizationMessageException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)
}

// To get details about a calling IAM user
//
// This example shows a request and response made with the credentials for a user named
// Alice in the AWS account 123456789012.
func ExampleSTS_GetCallerIdentity_shared00() {
	svc := sts.New(session.New())
	input := &sts.GetCallerIdentityInput{}

	result, err := svc.GetCallerIdentity(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)
}

// To get details about a calling user federated with AssumeRole
//
// This example shows a request and response made with temporary credentials created
// by AssumeRole. The name of the assumed role is my-role-name, and the RoleSessionName
// is set to my-role-session-name.
func ExampleSTS_GetCallerIdentity_shared01() {
	svc := sts.New(session.New())
	input := &sts.GetCallerIdentityInput{}

	result, err := svc.GetCallerIdentity(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)
}

// To get details about a calling user federated with GetFederationToken
//
// This example shows a request and response made with temporary credentials created
// by using GetFederationToken. The Name parameter is set to my-federated-user-name.
func ExampleSTS_GetCallerIdentity_shared02() {
	svc := sts.New(session.New())
	input := &sts.GetCallerIdentityInput{}

	result, err := svc.GetCallerIdentity(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)
}

// To get temporary credentials for a role by using GetFederationToken
//

func ExampleSTS_GetFederationToken_shared00() {
	svc := sts.New(session.New())
	input := &sts.GetFederationTokenInput{
		DurationSeconds: aws.Int64(3600),
		Name:            aws.String("testFedUserSession"),
		Policy:          aws.String("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Sid\":\"Stmt1\",\"Effect\":\"Allow\",\"Action\":\"s3:ListAllMyBuckets\",\"Resource\":\"*\"}]}"),
		Tags: []*sts.Tag{
			{
				Key:   aws.String("Project"),
				Value: aws.String("Pegasus"),
			},
			{
				Key:   aws.String("Cost-Center"),
				Value: aws.String("98765"),
			},
		},
	}

	result, err := svc.GetFederationToken(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case sts.ErrCodeMalformedPolicyDocumentException:
				fmt.Println(sts.ErrCodeMalformedPolicyDocumentException, aerr.Error())
			case sts.ErrCodePackedPolicyTooLargeException:
				fmt.Println(sts.ErrCodePackedPolicyTooLargeException, aerr.Error())
			case sts.ErrCodeRegionDisabledException:
				fmt.Println(sts.ErrCodeRegionDisabledException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)
}

// To get temporary credentials for an IAM user or an AWS account
//

func ExampleSTS_GetSessionToken_shared00() {
	svc := sts.New(session.New())
	input := &sts.GetSessionTokenInput{
		DurationSeconds: aws.Int64(3600),
		SerialNumber:    aws.String("YourMFASerialNumber"),
		TokenCode:       aws.String("123456"),
	}

	result, err := svc.GetSessionToken(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case sts.ErrCodeRegionDisabledException:
				fmt.Println(sts.ErrCodeRegionDisabledException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)
}
