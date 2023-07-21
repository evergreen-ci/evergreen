package evergreen

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/mongodb/grip"
)

// In order to add a new config section:
// 1. modify the struct in config.go to add whatever you need to add
// 2. add the struct to the ConfigSections variable below
// 3. add a copy of the struct you added in 1 to rest/model/admin.go and implement the
//    conversion methods. The property name must be exactly the same in the DB/API model
// 4. add it to MockConfig in testutil/config.go for testing, if desired

var ConfigRegistry *ConfigSectionRegistry

func init() {
	ConfigRegistry = &ConfigSectionRegistry{}
	if err := ConfigRegistry.reset(); err != nil {
		panic(errors.Wrap(err, "resetting registry"))
	}
}

type ConfigSectionRegistry struct {
	mu       sync.RWMutex
	sections map[string]ConfigSection
}

func (r *ConfigSectionRegistry) reset() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ConfigSections := []ConfigSection{
		&AlertsConfig{},
		&AmboyConfig{},
		&APIConfig{},
		&AuthConfig{},
		&BucketConfig{},
		&CedarConfig{},
		&CloudProviders{},
		&CommitQueueConfig{},
		&ContainerPoolsConfig{},
		&DataPipesConfig{},
		&HostInitConfig{},
		&HostJasperConfig{},
		&JiraConfig{},
		&LoggerConfig{},
		&NewRelicConfig{},
		&NotifyConfig{},
		&PodLifecycleConfig{},
		&ProjectCreationConfig{},
		&RepoTrackerConfig{},
		&SchedulerConfig{},
		&ServiceFlags{},
		&SlackConfig{},
		&SplunkConfig{},
		&UIConfig{},
		&Settings{},
		&JIRANotificationsConfig{},
		&TriggerConfig{},
		&SpawnHostConfig{},
		&TracerConfig{},
	}

	catcher := grip.NewSimpleCatcher()
	r.sections = make(map[string]ConfigSection)
	for _, section := range ConfigSections {
		catcher.Add(r.registerSection(section.SectionId(), section))
	}
	return catcher.Resolve()
}

func (r *ConfigSectionRegistry) registerSection(id string, section ConfigSection) error {
	if id == "" {
		return errors.New("cannot register a section with no ID")
	}
	if _, exists := r.sections[id]; exists {
		return errors.Errorf("section '%s' is already registered", id)
	}

	r.sections[id] = section
	return nil
}

func (r *ConfigSectionRegistry) populateSections(ctx context.Context) error {
	if err := r.reset(); err != nil {
		return errors.Wrap(err, "resetting registry")
	}

	var sectionIDs = make([]string, 0, len(r.sections))
	for sectionID := range r.sections {
		sectionIDs = append(sectionIDs, sectionID)
	}

	rawSections, err := getSectionsBSON(ctx, sectionIDs)
	if err != nil {
		return errors.Wrap(err, "getting raw sections")
	}
	catcher := grip.NewBasicCatcher()
	for _, rawSection := range rawSections {
		catcher.Add(r.unmarshalSection(rawSection))
	}

	return catcher.Resolve()
}

func (r *ConfigSectionRegistry) unmarshalSection(rawSection bson.Raw) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, err := rawSection.LookupErr("_id")
	if err != nil {
		return nil
	}
	idString, ok := id.StringValueOK()
	if !ok {
		return nil
	}
	section, ok := r.sections[idString]
	if !ok {
		return nil
	}
	return errors.Wrapf(bson.Unmarshal(rawSection, section), "unmarshaling section '%s'", idString)
}

func (r *ConfigSectionRegistry) GetSections() map[string]ConfigSection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sections
}

func (r *ConfigSectionRegistry) GetSection(id string) ConfigSection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sections[id]
}
