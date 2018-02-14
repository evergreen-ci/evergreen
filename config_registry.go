package evergreen

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mongodb/grip"
)

// In order to add a new config section:
// 1. modify the struct in config.go to add whatever you need to add
// 2. add the struct to the ConfigSections variable below
// 3. add a copy of the struct you added in 1 to rest/model/admin.go and implement the
//    conversion methods. The property name must be exactly the same in the DB/API model
// 4. add it to MockConfig in testutil/config.go for testing, if desired

var configRegistry *ConfigSectionRegistry

func init() {

	if err := resetRegistry(); err != nil {
		panic(fmt.Sprintf("error registering config sections: %s", err.Error()))
	}
}

type ConfigSectionRegistry struct {
	mu       sync.RWMutex
	sections map[string]ConfigSection
}

func resetRegistry() error {
	ConfigSections := []ConfigSection{
		&AlertsConfig{},
		&AmboyConfig{},
		&APIConfig{},
		&AuthConfig{},
		&CloudProviders{},
		&HostInitConfig{},
		&JiraConfig{},
		&LoggerConfig{},
		&NewRelicConfig{},
		&NotifyConfig{},
		&RepoTrackerConfig{},
		&SchedulerConfig{},
		&ServiceFlags{},
		&SlackConfig{},
		&UIConfig{},
	}

	configRegistry = newConfigSectionRegistry()
	catcher := grip.NewSimpleCatcher()

	for _, section := range ConfigSections {
		catcher.Add(configRegistry.registerSection(section.SectionId(), section))
	}

	return catcher.Resolve()
}

func newConfigSectionRegistry() *ConfigSectionRegistry {
	return &ConfigSectionRegistry{
		sections: map[string]ConfigSection{},
	}
}

func (r *ConfigSectionRegistry) registerSection(id string, section ConfigSection) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if id == "" {
		return errors.New("cannot register a section with no ID")
	}
	if _, exists := r.sections[id]; exists {
		return fmt.Errorf("section %s is already registered", id)
	}

	r.sections[id] = section
	return nil
}

func (r *ConfigSectionRegistry) getSections() map[string]ConfigSection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sections
}
