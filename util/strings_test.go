package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscapeReservedChars(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("asdf1234", EscapeJQLReservedChars("asdf1234"))
	assert.Equal(`q\\+h\\^`, EscapeJQLReservedChars("q+h^"))
	assert.Equal(`\\{\\}\\[\\]\\(\\)`, EscapeJQLReservedChars("{}[]()"))
	assert.Equal("", EscapeJQLReservedChars(""))
	assert.Equal(`\\+\\+\\+\\+`, EscapeJQLReservedChars("++++"))
}

func TestCoalesce(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("one", CoalesceString("", "one", "two"))
	assert.Equal("one", CoalesceStrings([]string{"", ""}, "", "one", "two"))
	assert.Equal("a", CoalesceStrings([]string{"", "a"}, "", "one", "two"))
	assert.Equal("one", CoalesceStrings(nil, "", "one", "two"))
	assert.Equal("", CoalesceStrings(nil, "", ""))
}

func TestAZToRegion(t *testing.T) {
	assert.Equal(t, "us-east-1", AZToRegion("us-east-1a"))
	assert.Equal(t, "us-west-2", AZToRegion("us-west-2b"))
	assert.Equal(t, "eu-west-1", AZToRegion("eu-west-1c"))
	assert.Equal(t, "", AZToRegion(""))
	assert.Equal(t, "", AZToRegion("a"))
}

func TestAWSAccountIDFromIAMARN(t *testing.T) {
	for name, tc := range map[string]struct {
		arn      string
		wantAcct string
		wantOK   bool
	}{
		"RoleARN": {
			arn:      "arn:aws:iam::123456789012:role/evg-upload",
			wantAcct: "123456789012",
			wantOK:   true,
		},
		"RootARN": {
			arn:      "arn:aws:iam::999999999999:root",
			wantAcct: "999999999999",
			wantOK:   true,
		},
		"InvalidService": {
			arn:    "arn:aws:s3:::mybucket",
			wantOK: false,
		},
		"TooShortAccount": {
			arn:    "arn:aws:iam::123:role/x",
			wantOK: false,
		},
		"Empty": {
			arn:    "",
			wantOK: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			acct, ok := AWSAccountIDFromIAMARN(tc.arn)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantAcct, acct)
			}
		})
	}
}
