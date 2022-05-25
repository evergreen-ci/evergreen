package operations

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/evergreen-ci/utility"
)

// prompt writes a prompt to the user on stdout, reads a newline-terminated response from stdin,
// and returns the result as a string.
func prompt(message string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(message + " ")
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

// confirm asks the user a yes/no question and returns true/false if they reply with y/yes/n/no.
// if defaultYes is true, allows user to just hit enter without typing an explicit yes.
func confirm(message string, defaultYes bool) bool {
	var reply string

	yes := []string{"y", "yes"}
	no := []string{"n", "no"}

	if defaultYes {
		yes = append(yes, "")
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
