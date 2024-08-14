package fakeparameter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeSSMClient(t *testing.T) {
	assert.Implements(t, (*parameterstore.SSMClient)(nil), NewFakeSSMClient())

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter){
		"PutParameterSucceeds": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			p := params[0]
			input := &ssm.PutParameterInput{
				Name:      utility.ToStringPtr(p.ID),
				Value:     utility.ToStringPtr(p.Value),
				Overwrite: utility.ToBoolPtr(true),
			}
			output, err := c.PutParameter(ctx, input)
			require.NoError(t, err)
			require.NotZero(t, output)

			dbParam, err := FindOneID(ctx, p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbParam)
			assert.Equal(t, p.ID, dbParam.ID)
			assert.Equal(t, p.Value, dbParam.Value)
			assert.WithinDuration(t, time.Now(), dbParam.LastUpdated, time.Second, "last updated time should be bumped")
		},
		"PutParameterFailsWithInvalidInput": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			input := &ssm.PutParameterInput{}
			output, err := c.PutParameter(ctx, input)
			assert.Error(t, err)
			assert.Zero(t, output)
		},
		"DeleteParametersWithSingleParameterSucceeds": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			paramNames := make([]string, 0, len(params))
			for _, p := range params {
				require.NoError(t, p.Insert(ctx))
				paramNames = append(paramNames, p.ID)
			}

			input := &ssm.DeleteParametersInput{
				Names: paramNames[0:1],
			}
			outputs, err := c.DeleteParameters(ctx, input)
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			assert.ElementsMatch(t, paramNames[0:1], outputs[0].DeletedParameters)

			dbParam, err := FindOneID(ctx, paramNames[0])
			assert.NoError(t, err)
			assert.Zero(t, dbParam)

			dbParams, err := FindByIDs(ctx, paramNames[1:]...)
			require.NoError(t, err)
			require.NotZero(t, dbParams)
			assert.Len(t, dbParams, len(paramNames[1:]))
		},
		"DeleteParametersWithMultipleParametersSucceeds": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			paramNames := make([]string, 0, len(params))
			for _, p := range params {
				require.NoError(t, p.Insert(ctx))
				paramNames = append(paramNames, p.ID)
			}

			input := &ssm.DeleteParametersInput{
				Names: paramNames[0:3],
			}
			outputs, err := c.DeleteParameters(ctx, input)
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			assert.ElementsMatch(t, paramNames[0:3], outputs[0].DeletedParameters)

			dbDeletedParams, err := FindByIDs(ctx, paramNames[0:3]...)
			assert.NoError(t, err)
			assert.Empty(t, dbDeletedParams)

			dbParams, err := FindByIDs(ctx, paramNames[3:]...)
			require.NoError(t, err)
			require.NotZero(t, dbParams)
			assert.Len(t, dbParams, len(paramNames[3:]))
		},
		"DeleteParametersWithNonexistentParametersReturnsInvalidParameters": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			paramNames := make([]string, 0, len(params))
			for _, p := range params {
				paramNames = append(paramNames, p.ID)
			}

			input := &ssm.DeleteParametersInput{
				Names: paramNames,
			}
			outputs, err := c.DeleteParameters(ctx, input)
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			assert.Empty(t, outputs[0].DeletedParameters, "should not return any deleted parameters because they are not present in fake parameter store")
			assert.ElementsMatch(t, paramNames, outputs[0].InvalidParameters, "nonexistent parameters should appear in invalid parameters")
		},
		"DeleteParametersNoopsWithNoNames": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			input := &ssm.DeleteParametersInput{}
			deletedNames, err := c.DeleteParametersSimple(ctx, input)
			assert.NoError(t, err)
			assert.Empty(t, deletedNames)
		},
		"GetParametersWithSingleParameterSucceeds": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			paramNames := make([]string, 0, len(params))
			for _, p := range params {
				require.NoError(t, p.Insert(ctx))
				paramNames = append(paramNames, p.ID)
			}

			input := &ssm.GetParametersInput{
				Names: paramNames[0:1],
			}
			outputs, err := c.GetParameters(ctx, input)
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			foundParams := outputs[0].Parameters
			require.Len(t, foundParams, 1)
			assert.Equal(t, params[0].ID, utility.FromStringPtr(foundParams[0].Name))
			assert.Equal(t, params[0].Value, utility.FromStringPtr(foundParams[0].Value))
			assert.Zero(t, utility.FromTimePtr(foundParams[0].LastModifiedDate), "should not bump last modified date for a get operation")
		},
		"GetParametersWithMultipleParametersSucceeds": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			paramNames := make([]string, 0, len(params))
			for _, p := range params {
				require.NoError(t, p.Insert(ctx))
				paramNames = append(paramNames, p.ID)
			}

			input := &ssm.GetParametersInput{
				Names: paramNames[0:3],
			}
			outputs, err := c.GetParameters(ctx, input)
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			foundParams := outputs[0].Parameters
			require.Len(t, foundParams, 3)
			for _, foundParam := range foundParams {
				name := utility.FromStringPtr(foundParam.Name)
				switch name {
				case params[0].ID:
					assert.Equal(t, utility.FromStringPtr(foundParam.Value), params[0].Value)
				case params[1].ID:
					assert.Equal(t, utility.FromStringPtr(foundParam.Value), params[1].Value)
				case params[2].ID:
					assert.Equal(t, utility.FromStringPtr(foundParam.Value), params[2].Value)
				default:
					assert.FailNow(t, "unexpected parameter '%s' returned in results", name)
				}
				assert.Zero(t, utility.FromTimePtr(foundParams[0].LastModifiedDate), "should not bump last modified date for a get operation")
			}
		},
		"GetParametersWithNonexistentParametersReturnsInvalidParameters": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			paramNames := make([]string, 0, len(params))
			for _, p := range params {
				paramNames = append(paramNames, p.ID)
			}

			input := &ssm.GetParametersInput{
				Names: paramNames,
			}
			outputs, err := c.GetParameters(ctx, input)
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			assert.Empty(t, outputs[0].Parameters, "should not return any parameters because they are not present in fake parameter store")
			assert.ElementsMatch(t, paramNames, outputs[0].InvalidParameters, "nonexistent parameters should appear in invalid parameters")
		},
		"GetParametersNoopsWithNoNames": func(ctx context.Context, t *testing.T, c *FakeSSMClient, params []FakeParameter) {
			input := &ssm.GetParametersInput{}
			outputs, err := c.GetParameters(ctx, input)
			assert.NoError(t, err)
			assert.Empty(t, outputs)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(Collection))

			var params []FakeParameter
			for i := 0; i < 5; i++ {
				params = append(params, FakeParameter{
					ID:    fmt.Sprintf("name-%d", i),
					Value: fmt.Sprintf("value-%d", i),
				})
			}

			tCase(ctx, t, NewFakeSSMClient(), params)
		})
	}
}
