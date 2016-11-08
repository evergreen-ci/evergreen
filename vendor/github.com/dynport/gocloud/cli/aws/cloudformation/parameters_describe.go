package cloudformation

import (
	"fmt"

	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/aws/cloudformation"
)

type ParametersDescribe struct {
	Name string `cli:"arg required"`
	Full bool   `cli:"opt --full"`
}

func (a *ParametersDescribe) Run() error {
	tpl := cloudformation.DescribeStacks{
		StackName: a.Name,
	}
	rsp, e := tpl.Execute(client)
	if e != nil {
		return e
	}
	stacks := rsp.DescribeStacksResult.Stacks
	if len(stacks) != 1 {
		return fmt.Errorf("expected 1 stack for %q, got %d", a.Name, len(stacks))
	}
	stack := stacks[0]
	t := gocli.NewTable()
	for _, p := range stack.Parameters {
		value := p.ParameterValue
		if len(value) > 64 && !a.Full {
			value = value[0:64] + "... (truncated)"
		}
		t.Add(p.ParameterKey, value)
	}
	fmt.Println(t)
	return nil
}
