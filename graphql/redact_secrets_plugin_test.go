package graphql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactFieldsInMap(t *testing.T) {
	t.Run("NoRedactedFields", func(t *testing.T) {
		fields := map[string]interface{}{
			"publicKey":       "public",
			"secret":          "secret",
			"servicePassword": "servicePassword",
			"vars":            "vars",
		}
		RedactFieldsInMap(fields, []string{})
		for _, value := range fields {
			require.NotEqual(t, "REDACTED", value, "Expected no fields to be redacted")
		}
	})
	t.Run("RedactedFields", func(t *testing.T) {
		fields := map[string]interface{}{
			"publicKey":       "somePublicKey",
			"secret":          "someSecret",
			"servicePassword": "someServicePassword",
			"someOtherField":  "someOtherField",
		}
		RedactFieldsInMap(fields, []string{
			"publicKey",
			"secret",
			"servicePassword",
		})
		require.Equal(t, "REDACTED", fields["publicKey"], "Expected publicKey to be redacted")
		require.Equal(t, "REDACTED", fields["secret"], "Expected secret to be redacted")
		require.Equal(t, "REDACTED", fields["servicePassword"], "Expected servicePassword to be redacted")
		require.Equal(t, "someOtherField", fields["someOtherField"], "Expected someOtherField to be unchanged")
	})
	t.Run("RedactedFieldsInNestedMap", func(t *testing.T) {
		fields := map[string]interface{}{
			"publicKey": "somePublicKey",
			"someInnerStruct": map[string]interface{}{
				"secret":  "someSecret",
				"service": "someService",
			},
		}
		RedactFieldsInMap(fields, []string{
			"secret",
		})
		require.Equal(t, "somePublicKey", fields["publicKey"], "Expected publicKey to be unchanged")
		innerStruct := fields["someInnerStruct"].(map[string]interface{})
		require.NotNil(t, innerStruct, "Expected someInnerStruct to be not nil")
		require.Equal(t, "REDACTED", innerStruct["secret"], "Expected secret to be redacted")
		require.Equal(t, "someService", innerStruct["service"], "Expected service to be unchanged")
	})
	t.Run("RedactsValuesInASlice", func(t *testing.T) {
		fields := map[string]interface{}{
			"publicKey": "somePublicKey",
			"someSlice": []interface{}{
				map[string]interface{}{
					"secret": "someSecret",
				},
				map[string]interface{}{
					"service": "someService",
				},
			},
		}
		RedactFieldsInMap(fields, []string{
			"secret",
		})
		require.Equal(t, "somePublicKey", fields["publicKey"], "Expected publicKey to be unchanged")
		someSlice := fields["someSlice"].([]interface{})
		require.NotNil(t, someSlice, "Expected someSlice to be not nil")
		value1 := someSlice[0].(map[string]interface{})
		require.Equal(t, "REDACTED", value1["secret"], "Expected secret to be redacted")
		value2 := someSlice[1].(map[string]interface{})
		require.Equal(t, "someService", value2["service"], "Expected service to be unchanged")
	})
}
