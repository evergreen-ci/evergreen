package graphql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactFieldsInMap(t *testing.T) {
	t.Run("NoRedactedFields", func(t *testing.T) {
		fields := map[string]any{
			"publicKey":       "public",
			"secret":          "secret",
			"servicePassword": "servicePassword",
			"vars":            "vars",
		}
		redactedFields := RedactFieldsInMap(fields, map[string]bool{})
		for _, value := range redactedFields {
			require.NotEqual(t, "REDACTED", value, "Expected no fields to be redacted")
		}
	})
	t.Run("RedactedFields", func(t *testing.T) {
		fields := map[string]any{
			"publicKey":       "somePublicKey",
			"secret":          "someSecret",
			"servicePassword": "someServicePassword",
			"someOtherField":  "someOtherField",
		}
		redactedFields := RedactFieldsInMap(fields, map[string]bool{
			"publicKey":       true,
			"secret":          true,
			"servicePassword": true,
		})
		require.Equal(t, "REDACTED", redactedFields["publicKey"], "Expected publicKey to be redacted")
		require.Equal(t, "REDACTED", redactedFields["secret"], "Expected secret to be redacted")
		require.Equal(t, "REDACTED", redactedFields["servicePassword"], "Expected servicePassword to be redacted")
		require.Equal(t, "someOtherField", redactedFields["someOtherField"], "Expected someOtherField to be unchanged")
	})
	t.Run("RedactedFieldsInNestedMap", func(t *testing.T) {
		fields := map[string]any{
			"publicKey": "somePublicKey",
			"someInnerStruct": map[string]any{
				"secret":  "someSecret",
				"service": "someService",
			},
		}
		redactedFields := RedactFieldsInMap(fields, map[string]bool{
			"secret": true,
		})
		require.Equal(t, "somePublicKey", redactedFields["publicKey"], "Expected publicKey to be unchanged")
		innerStruct := redactedFields["someInnerStruct"].(map[string]any)
		require.NotNil(t, innerStruct, "Expected someInnerStruct to be not nil")
		require.Equal(t, "REDACTED", innerStruct["secret"], "Expected secret to be redacted")
		require.Equal(t, "someService", innerStruct["service"], "Expected service to be unchanged")
	})
	t.Run("RedactsValuesInASlice", func(t *testing.T) {
		fields := map[string]any{
			"publicKey": "somePublicKey",
			"someSlice": []any{
				map[string]any{
					"secret": "someSecret",
				},
				map[string]any{
					"service": "someService",
				},
			},
		}
		redactedFields := RedactFieldsInMap(fields, map[string]bool{
			"secret": true,
		})
		require.Equal(t, "somePublicKey", redactedFields["publicKey"], "Expected publicKey to be unchanged")
		someSlice := redactedFields["someSlice"].([]any)
		require.NotNil(t, someSlice, "Expected someSlice to be not nil")
		value1 := someSlice[0].(map[string]any)
		require.Equal(t, "REDACTED", value1["secret"], "Expected secret to be redacted")
		value2 := someSlice[1].(map[string]any)
		require.Equal(t, "someService", value2["service"], "Expected service to be unchanged")
	})
}
