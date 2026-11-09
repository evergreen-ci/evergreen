package auth

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// KanopyDisplayName converts a first.last username into a "First Last" display name.
func KanopyDisplayName(username string) string {
	return cases.Title(language.English).String(strings.Join(strings.Split(username, "."), " "))
}

// NewKanopyUsernameFromEmail returns a function that returns the username in an email address,
// which is only trusted if the email address belongs to one of the expected domains. It returns
// an empty string for any other email address.
func NewKanopyUsernameFromEmail(expectedEmailDomains []string) func(string) string {
	return func(email string) string {
		domainStart := strings.LastIndex(email, "@")
		if domainStart <= 0 || !domainInAllowList(expectedEmailDomains, email[domainStart+1:]) {
			return ""
		}
		return email[:domainStart]
	}
}
