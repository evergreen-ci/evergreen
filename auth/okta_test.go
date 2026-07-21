package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMakeReconciliateID(t *testing.T) {
	for testName, testCase := range map[string]struct {
		expectedEmailDomains []string
		id                   string
		expected             string
	}{
		"StripsDomainWhenNoAllowListConfigured": {
			expectedEmailDomains: nil,
			id:                   "alice@a.com",
			expected:             "alice",
		},
		"StripsDomainWhenInAllowList": {
			expectedEmailDomains: []string{"a.com"},
			id:                   "alice@a.com",
			expected:             "alice",
		},
		"KeepsFullEmailWhenDomainNotInAllowList": {
			expectedEmailDomains: []string{"a.com"},
			id:                   "alice@b.com",
			expected:             "alice@b.com",
		},
		"ReturnsIDUnchangedWhenNoAtSign": {
			expectedEmailDomains: []string{"a.com"},
			id:                   "alice",
			expected:             "alice",
		},
	} {
		t.Run(testName, func(t *testing.T) {
			reconcile := makeReconciliateID(testCase.expectedEmailDomains)
			assert.Equal(t, testCase.expected, reconcile(testCase.id))
		})
	}
}
