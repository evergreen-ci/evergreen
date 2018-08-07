package shrub

import (
	"testing"
	"time"
)

func TestCommandDefinition(t *testing.T) {
	cases := map[string]func(*testing.T, *CommandDefinition){
		"ValidateIsAlwaysNil": func(t *testing.T, cmd *CommandDefinition) {
			assert(t, cmd.Validate() == nil, "validate should always be nil")
		},
		"ResolveIsSelf": func(t *testing.T, cmd *CommandDefinition) {
			assert(t, cmd.Resolve() == cmd, "resolve just implements the cmd interface")
		},
		"FunctionNameSetter": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Function("foo")

			assert(t, cmd.FunctionName == "foo", "setter mutates object")
			assert(t, c2 == cmd, "chainable")
		},
		"TypeSetter": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Type("foo")

			assert(t, cmd.ExecutionType == "foo", "setter mutates object")
			assert(t, c2 == cmd, "chainable")
		},
		"NameSetter": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Name("foo")
			assert(t, cmd.DisplayName == "foo", "setter mutates object")
			assert(t, c2 == cmd, "chainable")
		},
		"CommandSetter": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Command("foo")
			assert(t, cmd.CommandName == "foo", "setter mutates object")
			assert(t, c2 == cmd, "chainable")
		},
		"TimeoutSetterValidValues": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Timeout(time.Second)
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.TimeoutSecs == 1, "one second")

			c2 = cmd.Timeout(time.Minute)
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.TimeoutSecs == 60, "one minute")
		},
		"TimeoutWithSmallValue": func(t *testing.T, cmd *CommandDefinition) {
			assert(t, cmd.TimeoutSecs == 0, "defauilt")
			c2 := cmd.Timeout(time.Millisecond)
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.TimeoutSecs == 0, "zero")
		},
		"VariantSetterWithNoArgs": func(t *testing.T, cmd *CommandDefinition) {
			assert(t, len(cmd.RunVariants) == 0, "default")
			c2 := cmd.Variants()
			assert(t, len(cmd.RunVariants) == 0, "no change")
			assert(t, c2 == cmd, "chainable")
		},
		"VariantSetterWithDuplicateArgs": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Variants("foo", "foo", "foo")
			assert(t, len(cmd.RunVariants) == 3, "does not deduplicate")
			assert(t, c2 == cmd, "chainable")
		},
		"ReplaceNilParamsNoops": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.ReplaceParams(nil)
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Params == nil)
		},
		"ReplaceNilVarsNoops": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.ReplaceVars(nil)
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Vars == nil)
		},
		"ReplaceParamsNil": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Params = map[string]interface{}{"a": 1}
			assert(t, cmd.Params != nil)
			c2 := cmd.ReplaceParams(nil)
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Params == nil)
		},
		"ReplaceParams": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Params = map[string]interface{}{"a": 1}
			assert(t, cmd.Params != nil)
			assert(t, cmd.Params["a"] != nil)
			c2 := cmd.ReplaceParams(map[string]interface{}{"a": 2})
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Params["a"] != nil)
			assert(t, cmd.Params["a"] == 2)
		},
		"ReplaceVars": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Vars = map[string]string{"a": "foo"}
			assert(t, cmd.Vars != nil)
			assert(t, cmd.Vars["a"] != "")
			c2 := cmd.ReplaceVars(map[string]string{"a": "bar"})
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Vars["a"] != "")
			assert(t, cmd.Vars["a"] == "bar")
		},
		"ReplaceVarsNil": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Vars = map[string]string{"a": "b"}
			assert(t, cmd.Vars != nil)
			c2 := cmd.ReplaceVars(nil)
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Vars == nil)
		},
		"ResetNilParamsNoops": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.ResetParams()
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Params == nil, "is reset")
		},
		"ResetNilVarsNoops": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.ResetVars()
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Vars == nil, "is reset")
		},
		"ResetParams": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Params = map[string]interface{}{"a": 1}
			assert(t, cmd.Params != nil, "before reset")
			c2 := cmd.ResetParams()
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Params == nil, "is reset")
		},
		"ResetVars": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Vars = map[string]string{"a": "b"}
			assert(t, cmd.Vars != nil, "before reset")
			c2 := cmd.ResetVars()
			assert(t, c2 == cmd, "chainable")
			assert(t, cmd.Vars == nil, "is reset")
		},
		"AddFirstParam": func(t *testing.T, cmd *CommandDefinition) {
			assert(t, len(cmd.Params) == 0, "default value")
			c2 := cmd.Param("id", 1)
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Params) == 1, "with one value")
		},
		"AddSecondParam": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Param("id", 1).Param("second", 2)
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Params) == 2, "with one value")
		},
		"AddParamOverride": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Param("id", "foo").Param("id", "bar")
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Params) == 1, "override size")
			assert(t, cmd.Params["id"] == "bar", "override value", cmd.Vars["id"])
		},
		"AddFirstVar": func(t *testing.T, cmd *CommandDefinition) {
			assert(t, len(cmd.Vars) == 0, "default value")
			c2 := cmd.Var("id", "foo")
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Vars) == 1, "with one value")
		},
		"AddSecondVar": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Var("id", "foo").Var("second", "foo")
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Vars) == 2, "with one value")
		},
		"AddVarOverride": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.Var("id", "foo").Var("id", "bar")
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Vars) == 1, "override num")
			assert(t, cmd.Vars["id"] == "bar", "overridden value")
		},
		"ExtendVarAsSetter": func(t *testing.T, cmd *CommandDefinition) {
			c2 := cmd.ExtendVars(map[string]string{"a": "b"})
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Vars) == 1)
		},
		"ExtendParamAsSetter": func(t *testing.T, cmd *CommandDefinition) {
			assert(t, len(cmd.Params) == 0, "default")

			c2 := cmd.ExtendParams(map[string]interface{}{"a": true, "b": struct{}{}})
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Params) == 2, "values set")
		},
		"ExtendVarWithExistingOverride": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Vars = map[string]string{"a": "b"}
			c2 := cmd.ExtendVars(map[string]string{"a": "boo", "b": "eep"})
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Vars) == 2)
			assert(t, cmd.Vars["a"] == "boo")
		},
		"ExtendVarWithExisting": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Vars = map[string]string{"a": "b"}
			assert(t, len(cmd.Vars) == 1, "has existing value")
			c2 := cmd.ExtendVars(map[string]string{"b": "eep"})
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Vars) == 2, "extend appends")
			assert(t, cmd.Vars["a"] == "b")
		},
		"ExtendParamWithExisting": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Params = map[string]interface{}{"a": "b"}
			c2 := cmd.ExtendParams(map[string]interface{}{"a": "boo", "b": 42})
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Params) == 2)
			assert(t, cmd.Params["a"] == "boo")
		},
		"ExtendParamWithExistingOverride": func(t *testing.T, cmd *CommandDefinition) {
			cmd.Params = map[string]interface{}{"a": "b"}
			c2 := cmd.ExtendParams(map[string]interface{}{"a": "boo", "b": 42})
			assert(t, c2 == cmd, "chainable")
			assert(t, len(cmd.Params) == 2)
			assert(t, cmd.Params["a"] == "boo")
		},
	}

	for name, test := range cases {
		cmd := &CommandDefinition{}
		t.Run(name, func(t *testing.T) {
			test(t, cmd)
		})
	}
}

func TestCommandSequence(t *testing.T) {
	cases := map[string]func(*testing.T, *CommandSequence){
		"LengthMethod": func(t *testing.T, s *CommandSequence) {
			assert(t, s.Len() == 0, "is empty")
			*s = append(*s, &CommandDefinition{})

			assert(t, s.Len() == 1, "isn't empty")
		},
		"CommandChain": func(t *testing.T, s *CommandSequence) {
			cmd := s.Command()
			assert(t, cmd != nil, "creates cmd")
			assert(t, s.Len() == 1, "added")
		},
		"AppendZeroCase": func(t *testing.T, s *CommandSequence) {
			s.Append()
			assert(t, s.Len() == 0, "is empty")
		},
		"AppendSomething": func(t *testing.T, s *CommandSequence) {
			cmd := &CommandDefinition{CommandName: "foo"}
			s2 := s.Append(cmd)
			require(t, s.Len() == 1, "populated")
			require(t, s2.Len() == 1, "chain populated")
			assert(t, s == s2, "chain holds")
		},
		"AddNil": func(t *testing.T, s *CommandSequence) {
			defer expect(t, "calls resolve")

			s2 := s.Add(nil)
			assert(t, s2 != nil)
		},
		"AddCommand": func(t *testing.T, s *CommandSequence) {
			s2 := s.Add(CmdExec{})
			assert(t, s2 != nil)
			require(t, s.Len() == 1, "populated")
			assert(t, s == s2, "chain holds")
		},
		"AddErrorCommand": func(t *testing.T, s *CommandSequence) {
			defer expect(t, "bad command")

			s2 := s.Add(CmdS3Put{})
			assert(t, s2 != nil)
		},
		"ExtendNoop": func(t *testing.T, s *CommandSequence) {
			s2 := s.Extend()
			assert(t, s.Len() == 0, "is empty")
			assert(t, s2.Len() == 0, "is empty")
			assert(t, s == s2, "chain holds")
		},
		"ExtendWithNil": func(t *testing.T, s *CommandSequence) {
			defer expect(t, "calls resolve")
			s2 := s.Extend(nil, nil)
			assert(t, s2 != nil)
		},
		"ExtendWithBadCommand": func(t *testing.T, s *CommandSequence) {
			defer expect(t, "bad command")

			s2 := s.Extend(CmdS3Put{}, CmdS3Put{})
			assert(t, s2 != nil)
		},
		"ExtendWithMultipleValidCommands": func(t *testing.T, s *CommandSequence) {
			s2 := s.Extend(CmdExec{}, CmdExec{})
			assert(t, s2 != nil)
			require(t, s.Len() == 2, "populated")
			assert(t, s == s2, "chain holds")
		},
	}

	for name, test := range cases {
		s := &CommandSequence{}

		t.Run(name, func(t *testing.T) {
			test(t, s)
		})
	}
}
