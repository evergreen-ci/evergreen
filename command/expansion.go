package command

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"regexp"
	"strings"
)

var (
	expansionRegex = regexp.MustCompile("\\$\\{.*?\\}")
)

// Wrapper for an expansions map, with some utility functions.
type Expansions map[string]string

// Return a new Expansions object with all of the specified expansions present.
func NewExpansions(initMap map[string]string) *Expansions {
	exp := Expansions(map[string]string{})
	exp.Update(initMap)
	return &exp
}

// Update all of the specified keys in the expansions to point to the specified
// values.
func (self *Expansions) Update(newItems map[string]string) {
	for k, v := range newItems {
		self.Put(k, v)
	}
}

// Read a map of keys/values from the given file, and update the expansions
// to include them (overwriting any duplicates with the new value).
func (self *Expansions) UpdateFromYaml(filename string) error {
	filedata, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	newExpansions := make(map[string]string)
	err = yaml.Unmarshal(filedata, newExpansions)
	if err != nil {
		return err
	}
	self.Update(newExpansions)
	return nil
}

// Set a single value in the expansions.
func (self *Expansions) Put(expansion string, value string) {
	(*self)[expansion] = value
}

// Get a single value from the expansions.
// Return the value, or the empty string if the value is not present.
func (self *Expansions) Get(expansion string) string {
	if self.Exists(expansion) {
		return (*self)[expansion]
	}
	return ""
}

// Check if a value is present in the expansions.
func (self *Expansions) Exists(expansion string) bool {
	_, ok := (*self)[expansion]
	return ok
}

// Apply the expansions to a single string.
// Return the expanded string, or an error if the input string is malformed.
func (self *Expansions) ExpandString(toExpand string) (string, error) {
	// replace all expandable parts of the string
	malformedFound := false
	expanded := string(expansionRegex.ReplaceAllFunc([]byte(toExpand),
		func(matchByte []byte) []byte {

			match := string(matchByte)
			// trim off ${ and }
			match = match[2 : len(match)-1]

			// if there is a ${ within the match, then it is malformed (we
			// caught an unmatched ${)
			if strings.Contains(match, "${") {
				malformedFound = true
			}

			// parse into the name and default value
			var defaultVal string
			if idx := strings.Index(match, "|"); idx != -1 {
				defaultVal = match[idx+1:]
				match = match[0:idx]
			}

			// return the specified expansion, if it is present.
			if self.Exists(match) {
				return []byte(self.Get(match))
			}

			return []byte(defaultVal)
		}))

	if malformedFound || strings.Contains(expanded, "${") {
		return expanded, fmt.Errorf("The line \"%v\" is badly formed - it"+
			" contains an unclosed expansion", expanded)
	}

	return expanded, nil
}
