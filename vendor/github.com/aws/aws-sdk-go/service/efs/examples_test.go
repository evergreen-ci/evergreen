// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package efs_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/efs"
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

// To create a new file system
//
// This operation creates a new file system with the default generalpurpose performance
// mode.
func ExampleEFS_CreateFileSystem_shared00() {
	svc := efs.New(session.New())
	input := &efs.CreateFileSystemInput{
		CreationToken:   aws.String("tokenstring"),
		PerformanceMode: aws.String("generalPurpose"),
		Tags: []*efs.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String("MyFileSystem"),
			},
		},
	}

	result, err := svc.CreateFileSystem(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeFileSystemAlreadyExists:
				fmt.Println(efs.ErrCodeFileSystemAlreadyExists, aerr.Error())
			case efs.ErrCodeFileSystemLimitExceeded:
				fmt.Println(efs.ErrCodeFileSystemLimitExceeded, aerr.Error())
			case efs.ErrCodeInsufficientThroughputCapacity:
				fmt.Println(efs.ErrCodeInsufficientThroughputCapacity, aerr.Error())
			case efs.ErrCodeThroughputLimitExceeded:
				fmt.Println(efs.ErrCodeThroughputLimitExceeded, aerr.Error())
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

// To create a new mount target
//
// This operation creates a new mount target for an EFS file system.
func ExampleEFS_CreateMountTarget_shared00() {
	svc := efs.New(session.New())
	input := &efs.CreateMountTargetInput{
		FileSystemId: aws.String("fs-01234567"),
		SubnetId:     aws.String("subnet-1234abcd"),
	}

	result, err := svc.CreateMountTarget(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeFileSystemNotFound:
				fmt.Println(efs.ErrCodeFileSystemNotFound, aerr.Error())
			case efs.ErrCodeIncorrectFileSystemLifeCycleState:
				fmt.Println(efs.ErrCodeIncorrectFileSystemLifeCycleState, aerr.Error())
			case efs.ErrCodeMountTargetConflict:
				fmt.Println(efs.ErrCodeMountTargetConflict, aerr.Error())
			case efs.ErrCodeSubnetNotFound:
				fmt.Println(efs.ErrCodeSubnetNotFound, aerr.Error())
			case efs.ErrCodeNoFreeAddressesInSubnet:
				fmt.Println(efs.ErrCodeNoFreeAddressesInSubnet, aerr.Error())
			case efs.ErrCodeIpAddressInUse:
				fmt.Println(efs.ErrCodeIpAddressInUse, aerr.Error())
			case efs.ErrCodeNetworkInterfaceLimitExceeded:
				fmt.Println(efs.ErrCodeNetworkInterfaceLimitExceeded, aerr.Error())
			case efs.ErrCodeSecurityGroupLimitExceeded:
				fmt.Println(efs.ErrCodeSecurityGroupLimitExceeded, aerr.Error())
			case efs.ErrCodeSecurityGroupNotFound:
				fmt.Println(efs.ErrCodeSecurityGroupNotFound, aerr.Error())
			case efs.ErrCodeUnsupportedAvailabilityZone:
				fmt.Println(efs.ErrCodeUnsupportedAvailabilityZone, aerr.Error())
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

// To create a new tag
//
// This operation creates a new tag for an EFS file system.
func ExampleEFS_CreateTags_shared00() {
	svc := efs.New(session.New())
	input := &efs.CreateTagsInput{
		FileSystemId: aws.String("fs-01234567"),
		Tags: []*efs.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String("MyFileSystem"),
			},
		},
	}

	result, err := svc.CreateTags(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeFileSystemNotFound:
				fmt.Println(efs.ErrCodeFileSystemNotFound, aerr.Error())
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

// To delete a file system
//
// This operation deletes an EFS file system.
func ExampleEFS_DeleteFileSystem_shared00() {
	svc := efs.New(session.New())
	input := &efs.DeleteFileSystemInput{
		FileSystemId: aws.String("fs-01234567"),
	}

	result, err := svc.DeleteFileSystem(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeFileSystemNotFound:
				fmt.Println(efs.ErrCodeFileSystemNotFound, aerr.Error())
			case efs.ErrCodeFileSystemInUse:
				fmt.Println(efs.ErrCodeFileSystemInUse, aerr.Error())
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

// To delete a mount target
//
// This operation deletes a mount target.
func ExampleEFS_DeleteMountTarget_shared00() {
	svc := efs.New(session.New())
	input := &efs.DeleteMountTargetInput{
		MountTargetId: aws.String("fsmt-12340abc"),
	}

	result, err := svc.DeleteMountTarget(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeDependencyTimeout:
				fmt.Println(efs.ErrCodeDependencyTimeout, aerr.Error())
			case efs.ErrCodeMountTargetNotFound:
				fmt.Println(efs.ErrCodeMountTargetNotFound, aerr.Error())
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

// To delete tags for an EFS file system
//
// This operation deletes tags for an EFS file system.
func ExampleEFS_DeleteTags_shared00() {
	svc := efs.New(session.New())
	input := &efs.DeleteTagsInput{
		FileSystemId: aws.String("fs-01234567"),
		TagKeys: []*string{
			aws.String("Name"),
		},
	}

	result, err := svc.DeleteTags(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeFileSystemNotFound:
				fmt.Println(efs.ErrCodeFileSystemNotFound, aerr.Error())
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

// To describe an EFS file system
//
// This operation describes all of the EFS file systems in an account.
func ExampleEFS_DescribeFileSystems_shared00() {
	svc := efs.New(session.New())
	input := &efs.DescribeFileSystemsInput{}

	result, err := svc.DescribeFileSystems(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeFileSystemNotFound:
				fmt.Println(efs.ErrCodeFileSystemNotFound, aerr.Error())
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

// To describe the lifecycle configuration for a file system
//
// This operation describes a file system's LifecycleConfiguration. EFS lifecycle management
// uses the LifecycleConfiguration object to identify which files to move to the EFS
// Infrequent Access (IA) storage class.
func ExampleEFS_DescribeLifecycleConfiguration_shared00() {
	svc := efs.New(session.New())
	input := &efs.DescribeLifecycleConfigurationInput{
		FileSystemId: aws.String("fs-01234567"),
	}

	result, err := svc.DescribeLifecycleConfiguration(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeFileSystemNotFound:
				fmt.Println(efs.ErrCodeFileSystemNotFound, aerr.Error())
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

// To describe the security groups for a mount target
//
// This operation describes all of the security groups for a file system's mount target.
func ExampleEFS_DescribeMountTargetSecurityGroups_shared00() {
	svc := efs.New(session.New())
	input := &efs.DescribeMountTargetSecurityGroupsInput{
		MountTargetId: aws.String("fsmt-12340abc"),
	}

	result, err := svc.DescribeMountTargetSecurityGroups(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeMountTargetNotFound:
				fmt.Println(efs.ErrCodeMountTargetNotFound, aerr.Error())
			case efs.ErrCodeIncorrectMountTargetState:
				fmt.Println(efs.ErrCodeIncorrectMountTargetState, aerr.Error())
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

// To describe the mount targets for a file system
//
// This operation describes all of a file system's mount targets.
func ExampleEFS_DescribeMountTargets_shared00() {
	svc := efs.New(session.New())
	input := &efs.DescribeMountTargetsInput{
		FileSystemId: aws.String("fs-01234567"),
	}

	result, err := svc.DescribeMountTargets(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeFileSystemNotFound:
				fmt.Println(efs.ErrCodeFileSystemNotFound, aerr.Error())
			case efs.ErrCodeMountTargetNotFound:
				fmt.Println(efs.ErrCodeMountTargetNotFound, aerr.Error())
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

// To describe the tags for a file system
//
// This operation describes all of a file system's tags.
func ExampleEFS_DescribeTags_shared00() {
	svc := efs.New(session.New())
	input := &efs.DescribeTagsInput{
		FileSystemId: aws.String("fs-01234567"),
	}

	result, err := svc.DescribeTags(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeFileSystemNotFound:
				fmt.Println(efs.ErrCodeFileSystemNotFound, aerr.Error())
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

// To modify the security groups associated with a mount target for a file system
//
// This operation modifies the security groups associated with a mount target for a
// file system.
func ExampleEFS_ModifyMountTargetSecurityGroups_shared00() {
	svc := efs.New(session.New())
	input := &efs.ModifyMountTargetSecurityGroupsInput{
		MountTargetId: aws.String("fsmt-12340abc"),
		SecurityGroups: []*string{
			aws.String("sg-abcd1234"),
		},
	}

	result, err := svc.ModifyMountTargetSecurityGroups(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeMountTargetNotFound:
				fmt.Println(efs.ErrCodeMountTargetNotFound, aerr.Error())
			case efs.ErrCodeIncorrectMountTargetState:
				fmt.Println(efs.ErrCodeIncorrectMountTargetState, aerr.Error())
			case efs.ErrCodeSecurityGroupLimitExceeded:
				fmt.Println(efs.ErrCodeSecurityGroupLimitExceeded, aerr.Error())
			case efs.ErrCodeSecurityGroupNotFound:
				fmt.Println(efs.ErrCodeSecurityGroupNotFound, aerr.Error())
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

// Creates a new lifecycleconfiguration object for a file system
//
// This operation enables lifecycle management on a file system by creating a new LifecycleConfiguration
// object. A LifecycleConfiguration object defines when files in an Amazon EFS file
// system are automatically transitioned to the lower-cost EFS Infrequent Access (IA)
// storage class. A LifecycleConfiguration applies to all files in a file system.
func ExampleEFS_PutLifecycleConfiguration_shared00() {
	svc := efs.New(session.New())
	input := &efs.PutLifecycleConfigurationInput{
		FileSystemId: aws.String("fs-01234567"),
		LifecyclePolicies: []*efs.LifecyclePolicy{
			{
				TransitionToIA: aws.String("AFTER_30_DAYS"),
			},
		},
	}

	result, err := svc.PutLifecycleConfiguration(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case efs.ErrCodeBadRequest:
				fmt.Println(efs.ErrCodeBadRequest, aerr.Error())
			case efs.ErrCodeInternalServerError:
				fmt.Println(efs.ErrCodeInternalServerError, aerr.Error())
			case efs.ErrCodeFileSystemNotFound:
				fmt.Println(efs.ErrCodeFileSystemNotFound, aerr.Error())
			case efs.ErrCodeIncorrectFileSystemLifeCycleState:
				fmt.Println(efs.ErrCodeIncorrectFileSystemLifeCycleState, aerr.Error())
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
