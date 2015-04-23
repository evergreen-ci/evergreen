package cloudformation

import (
	"net/url"
	"strconv"
)

type BaseParameters struct {
	Capabilities    []string
	Parameters      []*StackParameter
	StackName       string
	StackPolicyBody string
	StackPolicyURL  string
	TemplateBody    string
	TemplateURL     string
}

type Values map[string]string

func (values Values) Encode() string {
	ret := url.Values{}
	for k, v := range values {
		if v != "" {
			ret.Add(k, v)
		}
	}
	return ret.Encode()
}

func (v Values) updateCapabilities(capabilities []string) {
	for i, c := range capabilities {
		v["Capabilities.member."+strconv.Itoa(i+1)] = c
	}
}

func (v Values) updateParameters(parameters []*StackParameter) {
	for i, p := range parameters {
		v["Parameters.member."+strconv.Itoa(i+1)+".ParameterKey"] = p.ParameterKey
		v["Parameters.member."+strconv.Itoa(i+1)+".ParameterValue"] = p.ParameterValue
	}
}

func (c *BaseParameters) values() Values {
	v := Values{
		"StackPolicyBody": c.StackPolicyBody,
		"StackPolicyURL":  c.StackPolicyURL,
		"TemplateBody":    c.TemplateBody,
		"TemplateURL":     c.TemplateURL,
		"StackName":       c.StackName,
	}
	v.updateCapabilities(c.Capabilities)
	v.updateParameters(c.Parameters)
	return v
}

type CreateStackParameters struct {
	BaseParameters
	DisableRollback  bool
	NotificationARNs []string
	OnFailure        string
	Tags             []*Tag
	TimeoutInMinutes int
}

func (c *CreateStackParameters) values() Values {
	v := c.BaseParameters.values()
	v["OnFailure"] = c.OnFailure
	if c.DisableRollback {
		v["DisableRollback"] = "true"
	}

	if c.TimeoutInMinutes > 0 {
		v["TimeoutInMinutes"] = strconv.Itoa(c.TimeoutInMinutes)
	}

	for i, arn := range c.NotificationARNs {
		v["NoNotificationARNs.member."+strconv.Itoa(i+1)] = arn
	}
	return v
}

type StackParameter struct {
	ParameterKey   string
	ParameterValue string
}

type CreateStackResponse struct {
	CreateStackResult *CreateStackResult `xml:"CreateStackResult"`
}

type CreateStackResult struct {
	StackId string `xml:"StackId"`
}

func (client *Client) CreateStack(params CreateStackParameters) (stackId string, e error) {
	r := &CreateStackResponse{}
	v := params.values()
	e = client.loadCloudFormationResource("CreateStack", v, r)
	if e != nil {
		return "", e
	}
	return r.CreateStackResult.StackId, nil
}
