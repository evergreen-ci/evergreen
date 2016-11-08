package cloudformation

import "fmt"

type UpdateStackParameters struct {
	BaseParameters
	StackPolicyDuringUpdateBody string
	StackPolicyDuringUpdateURL  string
	UsePreviousTemplate         bool
}

func (p *UpdateStackParameters) values() Values {
	v := p.BaseParameters.values()
	v["StackPolicyDuringUpdateBody"] = p.StackPolicyDuringUpdateBody
	v["StackPolicyDuringUpdateURL"] = p.StackPolicyDuringUpdateURL
	if p.UsePreviousTemplate {
		v["UsePreviousTemplate"] = "true"
	}
	return v
}

type UpdateStackResponse struct {
	UpdateStackResult *UpdateStackResult `xml:"UpdateStackResult"`
}

type UpdateStackResult struct {
	StackId string `xml:"StackId"`
}

type UpdateStack struct {
	Parameters                  []*StackParameter
	Capabilities                []string
	StackName                   string
	StackPolicyBody             string
	StackPolicyURL              string
	TemplateBody                string
	TemplateURL                 string
	StackPolicyDuringUpdateBody string
	StackPolicyDuringUpdateURL  string
	UsePreviousTemplate         bool
}

func (update *UpdateStack) values() Values {
	v := Values{
		"StackName":                   update.StackName,
		"StackPolicyBody":             update.StackPolicyBody,
		"StackPolicyURL":              update.StackPolicyURL,
		"TemplateBody":                update.TemplateBody,
		"TemplateURL":                 update.TemplateURL,
		"StackPolicyDuringUpdateBody": update.StackPolicyDuringUpdateBody,
		"StackPolicyDuringUpdateURL":  update.StackPolicyDuringUpdateURL,
	}
	if update.UsePreviousTemplate {
		v["UsePreviousTemplate"] = "true"
	}
	v.updateCapabilities(update.Capabilities)
	v.updateParameters(update.Parameters)
	return v
}

func (update *UpdateStack) Execute(client *Client) (*UpdateStackResponse, error) {
	r := &UpdateStackResponse{}
	e := client.loadCloudFormationResource("UpdateStack", update.values(), r)
	return r, e
}

const errorNoUpdate = "No updates are to be performed."

var ErrorNoUpdate = fmt.Errorf(errorNoUpdate)

func (c *Client) UpdateStack(params UpdateStackParameters) (stackId string, e error) {
	r := &UpdateStackResponse{}
	e = c.loadCloudFormationResource("UpdateStack", params.values(), r)
	if e != nil {
		dbg.Printf("error updating stack %T: %q", e, e)
		if e.Error() == errorNoUpdate {
			return "", ErrorNoUpdate
		}
		return "", e
	}
	return r.UpdateStackResult.StackId, nil
}
