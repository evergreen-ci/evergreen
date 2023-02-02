package operations

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/evergreen-ci/utility"
)

// prompt writes a prompt to the user on stdout, reads a newline-terminated response from stdin,
// and returns the result as a string.
func prompt(message string) string {
	oldState, _ := term.MakeRaw(int(os.Stdin.Fd()))
	defer term.Restore(int(os.Stdin.Fd()), oldState)
	terminal := term.NewTerminal(os.Stdin, fmt.Sprint(message+" "))
	text, _ := terminal.ReadLine()
	return strings.TrimSpace(text)
}

// confirm asks the user a yes/no question and returns true/false if they reply with y/yes/n/no.
// if defaultYes is true, allows user to just hit enter without typing an explicit yes.
// Otherwise we default no.
func confirm(message string, defaultYes bool) bool {
	var reply string

	yes := []string{"y", "yes"}
	no := []string{"n", "no"}

	if defaultYes {
		yes = append(yes, "")
		message = fmt.Sprintf("%s (Y/n)", message)
	} else {
		no = append(no, "")
		message = fmt.Sprintf("%s (y/N)", message)
	}

	for {
		reply = prompt(message)
		if utility.StringSliceContains(yes, strings.ToLower(reply)) {
			return true
		}
		if utility.StringSliceContains(no, strings.ToLower(reply)) {
			return false
		}
	}
}

func confirmWithMatchingString(message string, confirmation string) bool {
	var reply string
	for {
		reply = prompt(message)
		return confirmation == strings.TrimSpace(strings.ToLower(reply))
	}
}
