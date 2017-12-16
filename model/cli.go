package model

type ClientProjectConf struct {
	Name     string   `json:"name" yaml:"name,omitempty"`
	Default  bool     `json:"default" yaml:"default,omitempty"`
	Variants []string `json:"variants" yaml:"variants,omitempty"`
	Tasks    []string `json:"tasks" yaml:"tasks,omitempty"`
}

// CLISettings represents the data stored in the user's config file, by default
// located at ~/.evergreen.yml
type CLISettings struct {
	APIServerHost string              `json:"api_server_host" yaml:"api_server_host,omitempty"`
	UIServerHost  string              `json:"ui_server_host" yaml:"ui_server_host,omitempty"`
	APIKey        string              `json:"api_key" yaml:"api_key,omitempty"`
	User          string              `json:"user" yaml:"user,omitempty"`
	Projects      []ClientProjectConf `json:"projects" yaml:"projects,omitempty"`
	LoadedFrom    string              `json:"-" yaml:"-"`
}

func (s *CLISettings) FindDefaultProject() string {
	for _, p := range s.Projects {
		if p.Default {
			return p.Name
		}
	}
	return ""
}

func (s *CLISettings) FindDefaultVariants(project string) []string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.Variants
		}
	}
	return nil
}

func (s *CLISettings) SetDefaultVariants(project string, variants ...string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].Variants = variants
			return
		}
	}

	s.Projects = append(s.Projects, ClientProjectConf{project, true, variants, nil})
}

func (s *CLISettings) FindDefaultTasks(project string) []string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.Tasks
		}
	}
	return nil
}

func (s *CLISettings) SetDefaultTasks(project string, tasks ...string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].Tasks = tasks
			return
		}
	}

	s.Projects = append(s.Projects, ClientProjectConf{project, true, nil, tasks})
}

func (s *CLISettings) SetDefaultProject(name string) {
	var foundDefault bool
	for i, p := range s.Projects {
		if p.Name == name {
			s.Projects[i].Default = true
			foundDefault = true
		} else {
			s.Projects[i].Default = false
		}
	}

	if !foundDefault {
		s.Projects = append(s.Projects, ClientProjectConf{name, true, []string{}, []string{}})
	}
}
