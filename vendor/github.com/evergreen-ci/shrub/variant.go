package shrub

type Variant struct {
	BuildName        string                  `json:"name,omitempty"`
	BuildDisplayName string                  `json:"display_name,omitempty"`
	BatchTimeSecs    int                     `json:"batchtime,omitempty"`
	TaskSpecs        []TaskSpec              `json:"tasks,omitmepty"`
	DistroRunOn      []string                `json:"run_on,omitempty"`
	Expanisons       map[string]interface{}  `json:"expansions,omitempty"`
	DisplayTaskSpecs []DisplayTaskDefinition `json:"display_tasks,omitempty"`
}

type DisplayTaskDefinition struct {
	Name       string   `json:"name"`
	Components []string `json:"execution_tasks"`
}

type TaskSpec struct {
	Name     string   `json:"name"`
	Stepback bool     `json:"stepback,omitempty"`
	Distro   []string `json:"distros,omitempty"`
}

func (v *Variant) Name(id string) *Variant                         { v.BuildName = id; return v }
func (v *Variant) DisplayName(id string) *Variant                  { v.BuildDisplayName = id; return v }
func (v *Variant) RunOn(distro string) *Variant                    { v.DistroRunOn = []string{distro}; return v }
func (v *Variant) TaskSpec(spec TaskSpec) *Variant                 { v.TaskSpecs = append(v.TaskSpecs, spec); return v }
func (v *Variant) SetExpansions(m map[string]interface{}) *Variant { v.Expanisons = m; return v }
func (v *Variant) Expansion(k string, val interface{}) *Variant {
	if v.Expanisons == nil {
		v.Expanisons = make(map[string]interface{})
	}

	v.Expanisons[k] = val

	return v
}

func (v *Variant) AddTasks(name ...string) *Variant {
	for _, n := range name {
		if n == "" {
			continue
		}

		v.TaskSpecs = append(v.TaskSpecs, TaskSpec{
			Name: n,
		})
	}
	return v
}

func (v *Variant) DisplayTasks(def ...DisplayTaskDefinition) *Variant {
	v.DisplayTaskSpecs = append(v.DisplayTaskSpecs, def...)
	return v
}
