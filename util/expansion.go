package util

import (
	"os"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

var (
	expansionRegex = regexp.MustCompile(`\$\{.*?\}`)
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
func (exp *Expansions) Update(newItems map[string]string) {
	for k, v := range newItems {
		exp.Put(k, v)
	}
}

// Read a map of keys/values from the given file, and update the expansions
// to include them (overwriting any duplicates with the new value). It returns
// the updated keys from the Yaml.
func (exp *Expansions) UpdateFromYaml(filename string) ([]string, error) {
	filedata, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	newExpansions := make(map[string]string)
	err = yaml.Unmarshal(filedata, newExpansions)
	if err != nil {
		return nil, err
	}
	exp.Update(newExpansions)
	keys := []string{}
	for k := range newExpansions {
		keys = append(keys, k)
	}
	return keys, nil
}

// Set a single value in the expansions.
func (exp *Expansions) Put(expansion string, value string) {
	(*exp)[expansion] = value
}

// Get a single value from the expansions.
// Return the value, or the empty string if the value is not present.
func (exp *Expansions) Get(expansion string) string {
	return (*exp)[expansion]
}

// Check if a value is present in the expansions.
func (exp *Expansions) Exists(expansion string) bool {
	_, ok := (*exp)[expansion]
	return ok
}

// Remove deletes a value from the expansions.
func (exp *Expansions) Remove(expansion string) {
	delete(*exp, expansion)
}

// ExpandString applies the expansions to a single string.
// Return the expanded string, or an error if the input string is malformed.
func (exp *Expansions) ExpandString(toExpand string) (string, error) {
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

			// parse into the name and secondary value
			var secondaryValue string
			if idx := strings.Index(match, "|"); idx != -1 {
				secondaryValue = match[idx+1:]
				match = match[0:idx]
			}

			// determine if the expansion is required to have a value
			var requireVal bool
			if secondaryValue != "" && strings.HasSuffix(match, "!") {
				requireVal = true
				match = match[:len(match)-1]
			}

			// return the specified expansion, if it is present.
			if exp.Exists(match) {
				expVal := exp.Get(match)
				if !requireVal || (requireVal && expVal != "") {
					return []byte(expVal)
				}
			}

			// look for an expansion in the secondary value
			if strings.HasPrefix(secondaryValue, "*") {
				// trim off *
				secondaryValue = secondaryValue[1:]
				return []byte(exp.Get(secondaryValue))
			}

			// return the raw value if no expansion is found for either value
			return []byte(secondaryValue)
		}))

	if malformedFound || strings.Contains(expanded, "${") {
		return expanded, errors.Errorf("'%s' contains an unclosed expansion", toExpand)
	}

	return expanded, nil
}

func (exp *Expansions) Map() map[string]string {
	return *exp
}
