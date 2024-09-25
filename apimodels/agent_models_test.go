package apimodels

import (
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestAssumeRoleRequestValidate(t *testing.T) {
	testCases := []struct {
		desc        string
		req         AssumeRoleRequest
		errContains string
	}{
		{
			desc:        "ErrorIfNoRoleARN",
			req:         AssumeRoleRequest{},
			errContains: "must specify role ARN",
		},
		{
			desc: "ErrorIfNegativeDuration",
			req: AssumeRoleRequest{
				RoleARN:         "role",
				DurationSeconds: utility.ToInt32Ptr(-1),
			},
			errContains: "cannot specify a negative duration",
		},
		{
			desc: "SuccessWithNoDuration",
			req: AssumeRoleRequest{
				RoleARN: "role",
			},
		},
		{
			desc: "SuccessWithPositiveDuration",
			req: AssumeRoleRequest{
				RoleARN:         "role",
				DurationSeconds: utility.ToInt32Ptr(1),
			},
		},
		{
			desc: "SuccessWithAllFields",
			req: AssumeRoleRequest{
				RoleARN:         "role",
				DurationSeconds: utility.ToInt32Ptr(1),
				Policy:          utility.ToStringPtr("policy"),
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			err := tC.req.Validate()

			if tC.errContains != "" {
				assert.ErrorContains(t, err, tC.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
