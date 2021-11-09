package shrub

import (
	"fmt"
	"testing"
	"time"
)

func TestConfigTopLevelModifiers(t *testing.T) {
	cases := map[string]func(*testing.T, *Configuration){
		"AddMultipleTasks": func(t *testing.T, conf *Configuration) {
			assert(t, len(conf.Tasks) == 0, "is empty")
			t1 := conf.Task("one")
			assert(t, len(conf.Tasks) == 1, "has one")

			t2 := conf.Task("two")
			assert(t, len(conf.Tasks) == 2, "has two")

			t3 := conf.Task("one")
			assert(t, len(conf.Tasks) == 2, "still has two")

			assert(t, fmt.Sprint(t1) == fmt.Sprint(t3), "has one-per name")

			assert(t, fmt.Sprint(t1) != fmt.Sprint(t2), "not all are the same")
		},
		"AddSingleTaskRepeatedly": func(t *testing.T, conf *Configuration) {
			for i := 0; i < 10; i++ {
				conf.Task(fmt.Sprintf("task-%d", i))
			}
			assert(t, len(conf.Tasks) == 10, "has ten")
			for i := 0; i < 10; i++ {
				conf.Task(fmt.Sprintf("task-%d", i))
			}
			assert(t, len(conf.Tasks) == 10, "still has ten")
		},
		"AddOneTask": func(t *testing.T, conf *Configuration) {
			cases := make([]*Task, 10)
			for i := 0; i < 10; i++ {
				cases[i] = conf.Task("one")
			}
			different := conf.Task("two")

			for idx, task := range cases {
				if idx == 0 {
					continue
				}

				assert(t, cases[0] == task)
				assert(t, different != task)
			}
		},
		"AddMultipleTaskGroups": func(t *testing.T, conf *Configuration) {
			assert(t, len(conf.Groups) == 0, "is empty")
			t1 := conf.TaskGroup("one")
			assert(t, len(conf.Groups) == 1, "has one")

			t2 := conf.TaskGroup("two")
			assert(t, len(conf.Groups) == 2, "has two")

			t3 := conf.TaskGroup("one")
			assert(t, len(conf.Groups) == 2, "still has two")

			assert(t, fmt.Sprint(t1) == fmt.Sprint(t3), "has one-per name")

			assert(t, fmt.Sprint(t1) != fmt.Sprint(t2), "not all are the same")
		},
		"AddSingleTaskGroupRepeatedly": func(t *testing.T, conf *Configuration) {
			for i := 0; i < 10; i++ {
				conf.TaskGroup(fmt.Sprintf("task-%d", i))
			}
			assert(t, len(conf.Groups) == 10, "has ten")
			for i := 0; i < 10; i++ {
				conf.TaskGroup(fmt.Sprintf("task-%d", i))
			}
			assert(t, len(conf.Groups) == 10, "still has ten")
		},
		"AddOneTaskGroup": func(t *testing.T, conf *Configuration) {
			cases := make([]*TaskGroup, 10)
			for i := 0; i < 10; i++ {
				cases[i] = conf.TaskGroup("one")
			}
			different := conf.TaskGroup("two")

			for idx, task := range cases {
				if idx == 0 {
					continue
				}

				assert(t, cases[0] == task)
				assert(t, different != task)
			}
		},
		"AddMultipleFunctions": func(t *testing.T, conf *Configuration) {
			assert(t, len(conf.Functions) == 0, "is empty")
			t1 := conf.Function("one")
			t1.Command()
			assert(t, len(conf.Functions) == 1, "has one")

			t2 := conf.Function("two")
			assert(t, len(conf.Functions) == 2, "has two")

			t3 := conf.Function("one")
			assert(t, len(conf.Functions) == 2, "still has two")

			require(t, t1.Len() == t3.Len(), "has one-per name")

			assert(t, t1.Len() != t2.Len(), "not all are the same")
		},
		"AddSingleFunctionRepeatedly": func(t *testing.T, conf *Configuration) {
			for i := 0; i < 10; i++ {
				conf.Function(fmt.Sprintf("task-%d", i))
			}
			assert(t, len(conf.Functions) == 10, "has ten")
			for i := 0; i < 10; i++ {
				conf.Function(fmt.Sprintf("task-%d", i))
			}
			assert(t, len(conf.Functions) == 10, "still has ten")
		},
		"AddOneFunction": func(t *testing.T, conf *Configuration) {
			cases := make([]*CommandSequence, 10)
			for i := 0; i < 10; i++ {
				cases[i] = conf.Function("one")
			}

			different := conf.Function("two")

			for idx, function := range cases {
				if idx == 0 {
					continue
				}

				assert(t, cases[0] == function)
				assert(t, different != function)
			}
		},
		"AddMultipleVariants": func(t *testing.T, conf *Configuration) {
			assert(t, len(conf.Variants) == 0, "is empty")
			t1 := conf.Variant("one")
			assert(t, len(conf.Variants) == 1, "has one")

			t2 := conf.Variant("two")
			assert(t, len(conf.Variants) == 2, "has two")

			t3 := conf.Variant("one")
			assert(t, len(conf.Variants) == 2, "still has two")

			assert(t, fmt.Sprint(t1) == fmt.Sprint(t3), "has one-per name")

			assert(t, fmt.Sprint(t1) != fmt.Sprint(t2), "not all are the same")
		},
		"AddSingleVariantRepeatedly": func(t *testing.T, conf *Configuration) {
			for i := 0; i < 10; i++ {
				conf.Variant(fmt.Sprintf("task-%d", i))
			}
			assert(t, len(conf.Variants) == 10, "has ten")
			for i := 0; i < 10; i++ {
				conf.Variant(fmt.Sprintf("task-%d", i))
			}
			assert(t, len(conf.Variants) == 10, "still has ten")
		},
		"AddOneVariant": func(t *testing.T, conf *Configuration) {
			cases := make([]*Variant, 10)
			for i := 0; i < 10; i++ {
				cases[i] = conf.Variant("one")
			}
			different := conf.Variant("two")

			for idx, task := range cases {
				if idx == 0 {
					continue
				}

				assert(t, cases[0] == task)
				assert(t, different != task)
			}
		},
	}

	for name, test := range cases {
		conf := &Configuration{}
		t.Run(name, func(t *testing.T) {
			test(t, conf)
		})
	}
}

func TestHighLevelProjectSettings(t *testing.T) {
	cases := map[string]func(*testing.T, *Configuration){
		"SetExecTimeOut": func(t *testing.T, conf *Configuration) {
			assert(t, conf.ExecTimeoutSecs == 0, "has default zero value")

			conf.ExecTimeout(time.Millisecond)
			assert(t, conf.ExecTimeoutSecs == 0, "round to zero")

			conf.ExecTimeout(time.Minute)
			assert(t, conf.ExecTimeoutSecs == 60, "reasonable values are accepted")
		},
		"SetBatchTime": func(t *testing.T, conf *Configuration) {
			assert(t, conf.BatchTimeSecs == 0, "has default zero value")

			conf.BatchTime(time.Millisecond)
			assert(t, conf.BatchTimeSecs == 0, "round to zero")

			conf.BatchTime(time.Minute)
			assert(t, conf.BatchTimeSecs == 60, "reasonable values are accepted")
		},
		"SetValidCommandType": func(t *testing.T, conf *Configuration) {
			assert(t, conf.CommandType == "")

			for _, cmdType := range []string{"system", "setup", "task"} {
				conf.SetCommandType(cmdType)

				assert(t, conf.CommandType == cmdType)
			}
		},
		"SetInvalidCommandType": func(t *testing.T, _ *Configuration) {
			conf, err := BuildConfiguration(func(conf *Configuration) {
				conf.SetCommandType("foo")
			})
			assert(t, err != nil)
			assert(t, conf == nil)
		},
	}

	for name, test := range cases {
		conf := &Configuration{}
		t.Run(name, func(t *testing.T) {
			test(t, conf)
		})
	}
}
