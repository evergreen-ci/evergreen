package util

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpansions(t *testing.T) {
	getExpansions := func() *Expansions {
		return NewExpansions(map[string]string{
			"key1": "val1",
			"key2": "val2",
		})
	}

	t.Run("Get", func(t *testing.T) {
		expansions := getExpansions()
		assert.Equal(t, "val1", expansions.Get("key1"))
		assert.Equal(t, "val2", expansions.Get("key2"))
		assert.Equal(t, "", expansions.Get("key3"), "Empty string for missing expansion")
	})

	t.Run("Exists", func(t *testing.T) {
		expansions := getExpansions()
		assert.True(t, expansions.Exists("key1"))
		assert.True(t, expansions.Exists("key2"))
		assert.False(t, expansions.Exists("key3"), "False for missing expansion")
	})

	t.Run("Update", func(t *testing.T) {
		expansions := getExpansions()
		expansions.Update(map[string]string{
			"key2": "newval1",
			"key3": "newval2",
		})

		assert.Equal(t, "val1", expansions.Get("key1"))
		assert.Equal(t, "newval1", expansions.Get("key2"))
		assert.Equal(t, "newval2", expansions.Get("key3"))
	})

	t.Run("YamlUpdate", func(t *testing.T) {
		expansions := getExpansions()

		keys, err := expansions.UpdateFromYaml(filepath.Join(
			getDirectoryOfFile(), "testdata", "expansions.yml"),
		)
		require.NoError(t, err)
		assert.Equal(t, "1", expansions.Get("marge"))
		assert.Equal(t, "2", expansions.Get("lisa"))
		assert.Equal(t, "3", expansions.Get("bart"))
		assert.Equal(t, "4", expansions.Get("homer"))
		assert.Equal(t, "blah", expansions.Get("key1"))
		assert.Contains(t, keys, "marge")
		assert.Contains(t, keys, "lisa")
		assert.Contains(t, keys, "bart")
		assert.Contains(t, keys, "homer")
		assert.Contains(t, keys, "key1")
	})

}

func TestExpandString(t *testing.T) {
	for tName, tFunc := range map[string]struct {
		toExpand    string
		expanded    string
		expectedErr bool
	}{
		"UnchangedWithNoVariables": {
			toExpand: "hello hello hellohello",
			expanded: "hello hello hellohello",
		},
		"ExpandsMultipleVariables": {
			toExpand: "hello ${key1} ${key2} ${key3}",
			expanded: "hello val1 val2 val3",
		},
		"ExpandsWithDefaultValues": {
			toExpand: "hello ${key1|default1} ${key2|default2} ${key4|default4}",
			expanded: "hello val1 val2 default4",
		},
		"ExpandsWithMissingVariables": {
			toExpand: "hello ${key1|default1} ${key5} ${key6|default6}",
			expanded: "hello val1  default6",
		},
		"ExpandsDefaultWithExpansions": {
			toExpand: "hello ${key5|*key6} ${key5|*key2} ${key6|*key1}",
			expanded: "hello  val2 val1",
		},
		"ExpandsRequired": {
			toExpand: "hello ${empty|nope} ${empty!|yup}",
			expanded: "hello  yup",
		},
		"ExpandsRequiredWithExpansions": {
			toExpand: "hello ${empty|*nope} ${empty!|*key2}",
			expanded: "hello  val2",
		},
		"HandlesMalformedStrings": {
			toExpand:    "hello ${key1|default1}${key3}hello${ ${key4} ${key5|default5}hello",
			expectedErr: true,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			expansions := NewExpansions(map[string]string{
				"key1":  "val1",
				"key2":  "val2",
				"key3":  "val3",
				"empty": "",
			})

			expanded, err := expansions.ExpandString(tFunc.toExpand)
			if tFunc.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, expanded, tFunc.expanded)
			}
		})
	}
}
