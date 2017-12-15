package operations

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/evergreen-ci/evergreen/util"
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
		if util.StringSliceContains(yes, strings.ToLower(reply)) {
			return true
		}
		if util.StringSliceContains(no, strings.ToLower(reply)) {
			return false
		}
	}
}
