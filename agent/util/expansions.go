package util

import (
	"sync"

	"github.com/evergreen-ci/evergreen/util"
)

// DynamicExpansions wraps expansions for safe concurrent access as they are
// dynamically updated.
//
// This should be expanded to support better expansion handling during a task
// run.
type DynamicExpansions struct {
	util.Expansions
	mu sync.RWMutex

	// redact stores expansions that should be redacted.
	// These expansions can be from `expansion.update` or
	// from project commands that generate private expansions.
	// Since updates can be done dynamically and override previous
	// expansions that should be redacted, we need to store them
	// separately. Otherwise, previously redacted values can leak
	// if they are updated.
	redact []RedactInfo
}

type RedactInfo struct {
	Key   string
	Value string
}

func NewDynamicExpansions(e util.Expansions) *DynamicExpansions {
	return &DynamicExpansions{Expansions: e}
}

func (e *DynamicExpansions) Update(newExpansions map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Expansions.Update(newExpansions)
}

func (e *DynamicExpansions) Put(key, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Expansions.Put(key, value)
}

func (e *DynamicExpansions) UpdateFromYaml(filename string) ([]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.Expansions.UpdateFromYaml(filename)
}

func (e *DynamicExpansions) Get(key string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.Expansions.Get(key)
}

// Redact marks the expansion with given key for redaction.
func (e *DynamicExpansions) RedactKey(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if value := e.Expansions.Get(key); value != "" {
		e.redact = append(e.redact, RedactInfo{Key: key, Value: value})
	}
}

// GetRedacted gets the expansions that should be redacted.
func (e *DynamicExpansions) GetRedacted() []RedactInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.redact
}

// PutAndRedact puts the expansions followed by marking it for redaction.
func (e *DynamicExpansions) PutAndRedact(key, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Expansions.Put(key, value)
	if value != "" {
		e.redact = append(e.redact, RedactInfo{Key: key, Value: value})
	}
}

// UpdateFromYamlAndRedact updates the expansions from the given yaml file
// and then marks the expansions for redaction.
func (e *DynamicExpansions) UpdateFromYamlAndRedact(filename string) ([]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	keys, err := e.Expansions.UpdateFromYaml(filename)
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		if e.Expansions.Get(key) == "" {
			continue
		}
		e.redact = append(e.redact, RedactInfo{Key: key, Value: e.Expansions.Get(key)})
	}

	return keys, nil
}

// Remove deletes a value from the expansions.
func (e *DynamicExpansions) Remove(expansion string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Expansions.Remove(expansion)
}
