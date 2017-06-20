package distro

import (
	"fmt"
	"time"
	"math/rand"
)

// UserData validation formats
const (
	UserDataFormatFormURLEncoded = "x-www-form-urlencoded"
	UserDataFormatJSON           = "json"
	UserDataFormatYAML           = "yaml"
	// NameTimeFormat is the format in which to log times like instance start time.
	NameTimeFormat               = "20060102150405"
)

type Distro struct {
	Id               string                  `bson:"_id" json:"_id,omitempty" mapstructure:"_id,omitempty"`
	Arch             string                  `bson:"arch" json:"arch,omitempty" mapstructure:"arch,omitempty"`
	WorkDir          string                  `bson:"work_dir" json:"work_dir,omitempty" mapstructure:"work_dir,omitempty"`
	PoolSize         int                     `bson:"pool_size,omitempty" json:"pool_size,omitempty" mapstructure:"pool_size,omitempty" yaml:"poolsize"`
	Provider         string                  `bson:"provider" json:"provider,omitempty" mapstructure:"provider,omitempty"`
	ProviderSettings *map[string]interface{} `bson:"settings" json:"settings,omitempty" mapstructure:"settings,omitempty"`

	SetupAsSudo bool     `bson:"setup_as_sudo,omitempty" json:"setup_as_sudo,omitempty" mapstructure:"setup_as_sudo,omitempty"`
	Setup       string   `bson:"setup,omitempty" json:"setup,omitempty" mapstructure:"setup,omitempty"`
	Teardown    string   `bson:"teardown,omitempty" json:"teardown,omitempty" mapstructure:"teardown,omitempty"`
	User        string   `bson:"user,omitempty" json:"user,omitempty" mapstructure:"user,omitempty"`
	SSHKey      string   `bson:"ssh_key,omitempty" json:"ssh_key,omitempty" mapstructure:"ssh_key,omitempty"`
	SSHOptions  []string `bson:"ssh_options,omitempty" json:"ssh_options,omitempty" mapstructure:"ssh_options,omitempty"`
	UserData    UserData `bson:"user_data,omitempty" json:"user_data,omitempty" mapstructure:"user_data,omitempty"`

	SpawnAllowed bool        `bson:"spawn_allowed" json:"spawn_allowed,omitempty" mapstructure:"spawn_allowed,omitempty"`
	Expansions   []Expansion `bson:"expansions,omitempty" json:"expansions,omitempty" mapstructure:"expansions,omitempty"`
}

type ValidateFormat string

type UserData struct {
	File     string         `bson:"file,omitempty" json:"file,omitempty"`
	Validate ValidateFormat `bson:"validate,omitempty" json:"validate,omitempty"`
}

type Expansion struct {
	Key   string `bson:"key,omitempty" json:"key,omitempty"`
	Value string `bson:"value,omitempty" json:"value,omitempty"`
}

// GenerateName generates a unique instance name for a distro.
func (d *Distro) GenerateName() string {
	return "evg_" + d.Id + "_" + time.Now().Format(NameTimeFormat) +
		fmt.Sprintf("_%v", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
}
