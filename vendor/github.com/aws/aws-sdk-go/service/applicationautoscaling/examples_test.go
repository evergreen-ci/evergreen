// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package applicationautoscaling_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/applicationautoscaling"
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

// To delete a scaling policy
//
// This example deletes a scaling policy for the Amazon ECS service called web-app,
// which is running in the default cluster.
func ExampleApplicationAutoScaling_DeleteScalingPolicy_shared00() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.DeleteScalingPolicyInput{
		PolicyName:        aws.String("web-app-cpu-lt-25"),
		ResourceId:        aws.String("service/default/web-app"),
		ScalableDimension: aws.String("ecs:service:DesiredCount"),
		ServiceNamespace:  aws.String("ecs"),
	}

	result, err := svc.DeleteScalingPolicy(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeObjectNotFoundException:
				fmt.Println(applicationautoscaling.ErrCodeObjectNotFoundException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To deregister a scalable target
//
// This example deregisters a scalable target for an Amazon ECS service called web-app
// that is running in the default cluster.
func ExampleApplicationAutoScaling_DeregisterScalableTarget_shared00() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.DeregisterScalableTargetInput{
		ResourceId:        aws.String("service/default/web-app"),
		ScalableDimension: aws.String("ecs:service:DesiredCount"),
		ServiceNamespace:  aws.String("ecs"),
	}

	result, err := svc.DeregisterScalableTarget(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeObjectNotFoundException:
				fmt.Println(applicationautoscaling.ErrCodeObjectNotFoundException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To describe scalable targets
//
// This example describes the scalable targets for the ecs service namespace.
func ExampleApplicationAutoScaling_DescribeScalableTargets_shared00() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.DescribeScalableTargetsInput{
		ServiceNamespace: aws.String("ecs"),
	}

	result, err := svc.DescribeScalableTargets(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeInvalidNextTokenException:
				fmt.Println(applicationautoscaling.ErrCodeInvalidNextTokenException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To describe scaling activities for a scalable target
//
// This example describes the scaling activities for an Amazon ECS service called web-app
// that is running in the default cluster.
func ExampleApplicationAutoScaling_DescribeScalingActivities_shared00() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.DescribeScalingActivitiesInput{
		ResourceId:        aws.String("service/default/web-app"),
		ScalableDimension: aws.String("ecs:service:DesiredCount"),
		ServiceNamespace:  aws.String("ecs"),
	}

	result, err := svc.DescribeScalingActivities(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeInvalidNextTokenException:
				fmt.Println(applicationautoscaling.ErrCodeInvalidNextTokenException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To describe scaling policies
//
// This example describes the scaling policies for the ecs service namespace.
func ExampleApplicationAutoScaling_DescribeScalingPolicies_shared00() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.DescribeScalingPoliciesInput{
		ServiceNamespace: aws.String("ecs"),
	}

	result, err := svc.DescribeScalingPolicies(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeFailedResourceAccessException:
				fmt.Println(applicationautoscaling.ErrCodeFailedResourceAccessException, aerr.Error())
			case applicationautoscaling.ErrCodeInvalidNextTokenException:
				fmt.Println(applicationautoscaling.ErrCodeInvalidNextTokenException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To apply a target tracking scaling policy with a predefined metric specification
//
// The following example applies a target tracking scaling policy with a predefined
// metric specification to an Amazon ECS service called web-app in the default cluster.
// The policy keeps the average CPU utilization of the service at 75 percent, with scale-out
// and scale-in cooldown periods of 60 seconds.
func ExampleApplicationAutoScaling_PutScalingPolicy_shared00() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.PutScalingPolicyInput{
		PolicyName:        aws.String("cpu75-target-tracking-scaling-policy"),
		PolicyType:        aws.String("TargetTrackingScaling"),
		ResourceId:        aws.String("service/default/web-app"),
		ScalableDimension: aws.String("ecs:service:DesiredCount"),
		ServiceNamespace:  aws.String("ecs"),
		TargetTrackingScalingPolicyConfiguration: &applicationautoscaling.TargetTrackingScalingPolicyConfiguration{
			PredefinedMetricSpecification: &applicationautoscaling.PredefinedMetricSpecification{
				PredefinedMetricType: aws.String("ECSServiceAverageCPUUtilization"),
			},
			ScaleInCooldown:  aws.Int64(60),
			ScaleOutCooldown: aws.Int64(60),
			TargetValue:      aws.Float64(75.000000),
		},
	}

	result, err := svc.PutScalingPolicy(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeLimitExceededException:
				fmt.Println(applicationautoscaling.ErrCodeLimitExceededException, aerr.Error())
			case applicationautoscaling.ErrCodeObjectNotFoundException:
				fmt.Println(applicationautoscaling.ErrCodeObjectNotFoundException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeFailedResourceAccessException:
				fmt.Println(applicationautoscaling.ErrCodeFailedResourceAccessException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To apply a target tracking scaling policy with a customized metric specification
//
// The following example applies a target tracking scaling policy with a customized
// metric specification to an Amazon ECS service called web-app in the default cluster.
// The policy keeps the average utilization of the service at 75 percent, with scale-out
// and scale-in cooldown periods of 60 seconds.
func ExampleApplicationAutoScaling_PutScalingPolicy_shared01() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.PutScalingPolicyInput{
		PolicyName:        aws.String("cms75-target-tracking-scaling-policy"),
		PolicyType:        aws.String("TargetTrackingScaling"),
		ResourceId:        aws.String("service/default/web-app"),
		ScalableDimension: aws.String("ecs:service:DesiredCount"),
		ServiceNamespace:  aws.String("ecs"),
		TargetTrackingScalingPolicyConfiguration: &applicationautoscaling.TargetTrackingScalingPolicyConfiguration{
			CustomizedMetricSpecification: &applicationautoscaling.CustomizedMetricSpecification{
				Dimensions: []*applicationautoscaling.MetricDimension{
					{
						Name:  aws.String("MyOptionalMetricDimensionName"),
						Value: aws.String("MyOptionalMetricDimensionValue"),
					},
				},
				MetricName: aws.String("MyUtilizationMetric"),
				Namespace:  aws.String("MyNamespace"),
				Statistic:  aws.String("Average"),
				Unit:       aws.String("Percent"),
			},
			ScaleInCooldown:  aws.Int64(60),
			ScaleOutCooldown: aws.Int64(60),
			TargetValue:      aws.Float64(75.000000),
		},
	}

	result, err := svc.PutScalingPolicy(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeLimitExceededException:
				fmt.Println(applicationautoscaling.ErrCodeLimitExceededException, aerr.Error())
			case applicationautoscaling.ErrCodeObjectNotFoundException:
				fmt.Println(applicationautoscaling.ErrCodeObjectNotFoundException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeFailedResourceAccessException:
				fmt.Println(applicationautoscaling.ErrCodeFailedResourceAccessException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To apply a target tracking scaling policy for scale out only
//
// The following example applies a target tracking scaling policy to an Amazon ECS service
// called web-app in the default cluster. The policy is used to scale out the ECS service
// when the RequestCountPerTarget metric from the Application Load Balancer exceeds
// the threshold.
func ExampleApplicationAutoScaling_PutScalingPolicy_shared02() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.PutScalingPolicyInput{
		PolicyName:        aws.String("alb-scale-out-target-tracking-scaling-policy"),
		PolicyType:        aws.String("TargetTrackingScaling"),
		ResourceId:        aws.String("service/default/web-app"),
		ScalableDimension: aws.String("ecs:service:DesiredCount"),
		ServiceNamespace:  aws.String("ecs"),
		TargetTrackingScalingPolicyConfiguration: &applicationautoscaling.TargetTrackingScalingPolicyConfiguration{
			DisableScaleIn: aws.Bool(true),
			PredefinedMetricSpecification: &applicationautoscaling.PredefinedMetricSpecification{
				PredefinedMetricType: aws.String("ALBRequestCountPerTarget"),
				ResourceLabel:        aws.String("app/EC2Co-EcsEl-1TKLTMITMM0EO/f37c06a68c1748aa/targetgroup/EC2Co-Defau-LDNM7Q3ZH1ZN/6d4ea56ca2d6a18d"),
			},
			ScaleInCooldown:  aws.Int64(60),
			ScaleOutCooldown: aws.Int64(60),
			TargetValue:      aws.Float64(1000.000000),
		},
	}

	result, err := svc.PutScalingPolicy(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeLimitExceededException:
				fmt.Println(applicationautoscaling.ErrCodeLimitExceededException, aerr.Error())
			case applicationautoscaling.ErrCodeObjectNotFoundException:
				fmt.Println(applicationautoscaling.ErrCodeObjectNotFoundException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeFailedResourceAccessException:
				fmt.Println(applicationautoscaling.ErrCodeFailedResourceAccessException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To apply a step scaling policy to an Amazon ECS service
//
// This example applies a step scaling policy to an Amazon ECS service called web-app
// in the default cluster. The policy increases the desired count of the service by
// 200%, with a cool down period of 60 seconds.
func ExampleApplicationAutoScaling_PutScalingPolicy_shared03() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.PutScalingPolicyInput{
		PolicyName:        aws.String("web-app-cpu-gt-75"),
		PolicyType:        aws.String("StepScaling"),
		ResourceId:        aws.String("service/default/web-app"),
		ScalableDimension: aws.String("ecs:service:DesiredCount"),
		ServiceNamespace:  aws.String("ecs"),
		StepScalingPolicyConfiguration: &applicationautoscaling.StepScalingPolicyConfiguration{
			AdjustmentType: aws.String("PercentChangeInCapacity"),
			Cooldown:       aws.Int64(60),
			StepAdjustments: []*applicationautoscaling.StepAdjustment{
				{
					MetricIntervalLowerBound: aws.Float64(0.000000),
					ScalingAdjustment:        aws.Int64(200),
				},
			},
		},
	}

	result, err := svc.PutScalingPolicy(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeLimitExceededException:
				fmt.Println(applicationautoscaling.ErrCodeLimitExceededException, aerr.Error())
			case applicationautoscaling.ErrCodeObjectNotFoundException:
				fmt.Println(applicationautoscaling.ErrCodeObjectNotFoundException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeFailedResourceAccessException:
				fmt.Println(applicationautoscaling.ErrCodeFailedResourceAccessException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To apply a step scaling policy to an Amazon EC2 Spot fleet
//
// This example applies a step scaling policy to an Amazon EC2 Spot fleet. The policy
// increases the target capacity of the spot fleet by 200%, with a cool down period
// of 180 seconds.",
func ExampleApplicationAutoScaling_PutScalingPolicy_shared04() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.PutScalingPolicyInput{
		PolicyName:        aws.String("fleet-cpu-gt-75"),
		PolicyType:        aws.String("StepScaling"),
		ResourceId:        aws.String("spot-fleet-request/sfr-45e69d8a-be48-4539-bbf3-3464e99c50c3"),
		ScalableDimension: aws.String("ec2:spot-fleet-request:TargetCapacity"),
		ServiceNamespace:  aws.String("ec2"),
		StepScalingPolicyConfiguration: &applicationautoscaling.StepScalingPolicyConfiguration{
			AdjustmentType: aws.String("PercentChangeInCapacity"),
			Cooldown:       aws.Int64(180),
			StepAdjustments: []*applicationautoscaling.StepAdjustment{
				{
					MetricIntervalLowerBound: aws.Float64(0.000000),
					ScalingAdjustment:        aws.Int64(200),
				},
			},
		},
	}

	result, err := svc.PutScalingPolicy(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeLimitExceededException:
				fmt.Println(applicationautoscaling.ErrCodeLimitExceededException, aerr.Error())
			case applicationautoscaling.ErrCodeObjectNotFoundException:
				fmt.Println(applicationautoscaling.ErrCodeObjectNotFoundException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeFailedResourceAccessException:
				fmt.Println(applicationautoscaling.ErrCodeFailedResourceAccessException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To register an ECS service as a scalable target
//
// This example registers a scalable target from an Amazon ECS service called web-app
// that is running on the default cluster, with a minimum desired count of 1 task and
// a maximum desired count of 10 tasks.
func ExampleApplicationAutoScaling_RegisterScalableTarget_shared00() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.RegisterScalableTargetInput{
		MaxCapacity:       aws.Int64(10),
		MinCapacity:       aws.Int64(1),
		ResourceId:        aws.String("service/default/web-app"),
		ScalableDimension: aws.String("ecs:service:DesiredCount"),
		ServiceNamespace:  aws.String("ecs"),
	}

	result, err := svc.RegisterScalableTarget(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeLimitExceededException:
				fmt.Println(applicationautoscaling.ErrCodeLimitExceededException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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

// To register an EC2 Spot fleet as a scalable target
//
// This example registers a scalable target from an Amazon EC2 Spot fleet with a minimum
// target capacity of 1 and a maximum of 10.
func ExampleApplicationAutoScaling_RegisterScalableTarget_shared01() {
	svc := applicationautoscaling.New(session.New())
	input := &applicationautoscaling.RegisterScalableTargetInput{
		MaxCapacity:       aws.Int64(10),
		MinCapacity:       aws.Int64(1),
		ResourceId:        aws.String("spot-fleet-request/sfr-45e69d8a-be48-4539-bbf3-3464e99c50c3"),
		ScalableDimension: aws.String("ec2:spot-fleet-request:TargetCapacity"),
		ServiceNamespace:  aws.String("ec2"),
	}

	result, err := svc.RegisterScalableTarget(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case applicationautoscaling.ErrCodeValidationException:
				fmt.Println(applicationautoscaling.ErrCodeValidationException, aerr.Error())
			case applicationautoscaling.ErrCodeLimitExceededException:
				fmt.Println(applicationautoscaling.ErrCodeLimitExceededException, aerr.Error())
			case applicationautoscaling.ErrCodeConcurrentUpdateException:
				fmt.Println(applicationautoscaling.ErrCodeConcurrentUpdateException, aerr.Error())
			case applicationautoscaling.ErrCodeInternalServiceException:
				fmt.Println(applicationautoscaling.ErrCodeInternalServiceException, aerr.Error())
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
