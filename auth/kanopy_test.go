package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKanopyDisplayName(t *testing.T) {
	assert.Equal(t, "Sam Smith", KanopyDisplayName("sam.smith"))
	assert.Equal(t, "Samsmith", KanopyDisplayName("samsmith"))
}

func TestNewKanopyUsernameFromEmail(t *testing.T) {
	usernameFromEmail := NewKanopyUsernameFromEmail([]string{"mongodb.com", "10gen.com"})

	for _, tCase := range []struct {
		name     string
		email    string
		expected string
	}{
		{name: "ExpectedDomainShouldReturnUsername", email: "sam.smith@mongodb.com", expected: "sam.smith"},
		{name: "ExpectedDomainShouldBeCaseInsensitive", email: "sam.smith@MongoDB.com", expected: "sam.smith"},
		{name: "OtherExpectedDomainShouldReturnUsername", email: "sam.smith@10gen.com", expected: "sam.smith"},
		{name: "UnexpectedDomainShouldReturnEmpty", email: "sam.smith@example.com"},
		{name: "SubdomainOfExpectedDomainShouldReturnEmpty", email: "sam.smith@evil.mongodb.com"},
		{name: "MissingLocalPartShouldReturnEmpty", email: "@mongodb.com"},
		{name: "NotAnEmailShouldReturnEmpty", email: "sam.smith"},
		{name: "EmptyEmailShouldReturnEmpty"},
	} {
		t.Run(tCase.name, func(t *testing.T) {
			assert.Equal(t, tCase.expected, usernameFromEmail(tCase.email))
		})
	}

	t.Run("NoExpectedDomainsShouldReturnEmpty", func(t *testing.T) {
		assert.Empty(t, NewKanopyUsernameFromEmail(nil)("sam.smith@mongodb.com"))
	})
}
