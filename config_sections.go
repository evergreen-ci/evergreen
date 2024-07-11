package evergreen

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/mongodb/grip"
)

// In order to add a new config section:
//  1. modify the struct in config.go to add whatever you need to add
//  2. add the struct to the ConfigSections constructor below
//  3. add a copy of the struct you added in 1 to rest/model/admin.go and implement the
//     conversion methods. The property name must be exactly the same in the DB/API model
//  4. add it to MockConfig in testutil/config.go for testing, if desired

type ConfigSections struct {
	Sections map[string]ConfigSection
}

func NewConfigSections() ConfigSections {
	sections := []ConfigSection{
		&AmboyConfig{},
		&APIConfig{},
		&AuthConfig{},
		&BucketsConfig{},
		&CedarConfig{},
		&CloudProviders{},
		&CommitQueueConfig{},
		&ContainerPoolsConfig{},
		&HostInitConfig{},
		&HostJasperConfig{},
		&JiraConfig{},
		&LoggerConfig{},
		&NewRelicConfig{},
		&NotifyConfig{},
		&PodLifecycleConfig{},
		&ProjectCreationConfig{},
		&RepoTrackerConfig{},
		&RuntimeEnvironmentsConfig{},
		&SchedulerConfig{},
		&ServiceFlags{},
		&SlackConfig{},
		&SleepScheduleConfig{},
		&SplunkConfig{},
		&UIConfig{},
		&Settings{},
		&JIRANotificationsConfig{},
		&TaskLimitsConfig{},
		&TriggerConfig{},
		&SpawnHostConfig{},
		&TracerConfig{},
		&GitHubCheckRunConfig{},
	}

	sectionMap := make(map[string]ConfigSection, len(sections))
	for _, section := range sections {
		sectionMap[section.SectionId()] = section
	}
	return ConfigSections{Sections: sectionMap}
}

func (c *ConfigSections) populateSections(ctx context.Context) error {
	if err := c.getSSMParameters(ctx); err != nil && !errors.Is(err, ssmDisabledErr) {
		return errors.Wrap(err, "getting SSM parameters")
	}

	// A parameter set in the database overrides the same parameter set in SSM.
	return errors.Wrap(c.getDBParameters(ctx), "getting database parameters")
}

func (c *ConfigSections) getSSMParameters(ctx context.Context) error {
	ssmSections, err := getAllParameters(ctx)
	if err != nil {
		return errors.Wrap(err, "getting parameters from SSM")
	}

	catcher := grip.NewBasicCatcher()
	for sectionID, value := range ssmSections {
		section, ok := c.Sections[sectionID]
		if !ok {
			continue
		}
		catcher.Wrapf(json.Unmarshal([]byte(value), section), "unmarshalling SSM section ID '%s'", sectionID)
	}

	return catcher.Resolve()
}

func (c *ConfigSections) getDBParameters(ctx context.Context) error {
	sectionIDs := make([]string, 0, len(c.Sections))
	for sectionID := range c.Sections {
		sectionIDs = append(sectionIDs, sectionID)
	}

	rawSections, err := getSectionsBSON(ctx, sectionIDs)
	if err != nil {
		return errors.Wrap(err, "getting raw sections")
	}
	catcher := grip.NewBasicCatcher()
	for _, rawSection := range rawSections {
		catcher.Add(c.unmarshallSection(rawSection))
	}

	return catcher.Resolve()
}

func (c *ConfigSections) unmarshallSection(rawSection bson.Raw) error {
	id, err := rawSection.LookupErr("_id")
	if err != nil {
		return nil
	}
	idString, ok := id.StringValueOK()
	if !ok {
		return nil
	}
	section, ok := c.Sections[idString]
	if !ok {
		return nil
	}
	return errors.Wrapf(bson.Unmarshal(rawSection, section), "unmarshalling section '%s'", idString)
}
