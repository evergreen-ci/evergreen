/*
Foliage Web Services

Foliage web services, owner: DevProd Services & Integrations team

API version: 1.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package fws

import (
	"encoding/json"
	"bytes"
	"fmt"
)

// checks if the TeamData type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &TeamData{}

// TeamData This dataclass holds the team data for a specific team.
type TeamData struct {
	TeamName string `json:"team_name"`
	JiraProject string `json:"jira_project"`
	SlackChannelId NullableString `json:"slack_channel_id"`
	EvergreenTagName string `json:"evergreen_tag_name"`
	TriageTeamName NullableString `json:"triage_team_name,omitempty"`
	SlackGroupId NullableString `json:"slack_group_id,omitempty"`
	TriagedTeamNames []string `json:"triaged_team_names,omitempty"`
}

type _TeamData TeamData

// NewTeamData instantiates a new TeamData object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewTeamData(teamName string, jiraProject string, slackChannelId NullableString, evergreenTagName string) *TeamData {
	this := TeamData{}
	this.TeamName = teamName
	this.JiraProject = jiraProject
	this.SlackChannelId = slackChannelId
	this.EvergreenTagName = evergreenTagName
	return &this
}

// NewTeamDataWithDefaults instantiates a new TeamData object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewTeamDataWithDefaults() *TeamData {
	this := TeamData{}
	return &this
}

// GetTeamName returns the TeamName field value
func (o *TeamData) GetTeamName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.TeamName
}

// GetTeamNameOk returns a tuple with the TeamName field value
// and a boolean to check if the value has been set.
func (o *TeamData) GetTeamNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.TeamName, true
}

// SetTeamName sets field value
func (o *TeamData) SetTeamName(v string) {
	o.TeamName = v
}

// GetJiraProject returns the JiraProject field value
func (o *TeamData) GetJiraProject() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.JiraProject
}

// GetJiraProjectOk returns a tuple with the JiraProject field value
// and a boolean to check if the value has been set.
func (o *TeamData) GetJiraProjectOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.JiraProject, true
}

// SetJiraProject sets field value
func (o *TeamData) SetJiraProject(v string) {
	o.JiraProject = v
}

// GetSlackChannelId returns the SlackChannelId field value
// If the value is explicit nil, the zero value for string will be returned
func (o *TeamData) GetSlackChannelId() string {
	if o == nil || o.SlackChannelId.Get() == nil {
		var ret string
		return ret
	}

	return *o.SlackChannelId.Get()
}

// GetSlackChannelIdOk returns a tuple with the SlackChannelId field value
// and a boolean to check if the value has been set.
// NOTE: If the value is an explicit nil, `nil, true` will be returned
func (o *TeamData) GetSlackChannelIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return o.SlackChannelId.Get(), o.SlackChannelId.IsSet()
}

// SetSlackChannelId sets field value
func (o *TeamData) SetSlackChannelId(v string) {
	o.SlackChannelId.Set(&v)
}

// GetEvergreenTagName returns the EvergreenTagName field value
func (o *TeamData) GetEvergreenTagName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.EvergreenTagName
}

// GetEvergreenTagNameOk returns a tuple with the EvergreenTagName field value
// and a boolean to check if the value has been set.
func (o *TeamData) GetEvergreenTagNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.EvergreenTagName, true
}

// SetEvergreenTagName sets field value
func (o *TeamData) SetEvergreenTagName(v string) {
	o.EvergreenTagName = v
}

// GetTriageTeamName returns the TriageTeamName field value if set, zero value otherwise (both if not set or set to explicit null).
func (o *TeamData) GetTriageTeamName() string {
	if o == nil || IsNil(o.TriageTeamName.Get()) {
		var ret string
		return ret
	}
	return *o.TriageTeamName.Get()
}

// GetTriageTeamNameOk returns a tuple with the TriageTeamName field value if set, nil otherwise
// and a boolean to check if the value has been set.
// NOTE: If the value is an explicit nil, `nil, true` will be returned
func (o *TeamData) GetTriageTeamNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return o.TriageTeamName.Get(), o.TriageTeamName.IsSet()
}

// HasTriageTeamName returns a boolean if a field has been set.
func (o *TeamData) HasTriageTeamName() bool {
	if o != nil && o.TriageTeamName.IsSet() {
		return true
	}

	return false
}

// SetTriageTeamName gets a reference to the given NullableString and assigns it to the TriageTeamName field.
func (o *TeamData) SetTriageTeamName(v string) {
	o.TriageTeamName.Set(&v)
}
// SetTriageTeamNameNil sets the value for TriageTeamName to be an explicit nil
func (o *TeamData) SetTriageTeamNameNil() {
	o.TriageTeamName.Set(nil)
}

// UnsetTriageTeamName ensures that no value is present for TriageTeamName, not even an explicit nil
func (o *TeamData) UnsetTriageTeamName() {
	o.TriageTeamName.Unset()
}

// GetSlackGroupId returns the SlackGroupId field value if set, zero value otherwise (both if not set or set to explicit null).
func (o *TeamData) GetSlackGroupId() string {
	if o == nil || IsNil(o.SlackGroupId.Get()) {
		var ret string
		return ret
	}
	return *o.SlackGroupId.Get()
}

// GetSlackGroupIdOk returns a tuple with the SlackGroupId field value if set, nil otherwise
// and a boolean to check if the value has been set.
// NOTE: If the value is an explicit nil, `nil, true` will be returned
func (o *TeamData) GetSlackGroupIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return o.SlackGroupId.Get(), o.SlackGroupId.IsSet()
}

// HasSlackGroupId returns a boolean if a field has been set.
func (o *TeamData) HasSlackGroupId() bool {
	if o != nil && o.SlackGroupId.IsSet() {
		return true
	}

	return false
}

// SetSlackGroupId gets a reference to the given NullableString and assigns it to the SlackGroupId field.
func (o *TeamData) SetSlackGroupId(v string) {
	o.SlackGroupId.Set(&v)
}
// SetSlackGroupIdNil sets the value for SlackGroupId to be an explicit nil
func (o *TeamData) SetSlackGroupIdNil() {
	o.SlackGroupId.Set(nil)
}

// UnsetSlackGroupId ensures that no value is present for SlackGroupId, not even an explicit nil
func (o *TeamData) UnsetSlackGroupId() {
	o.SlackGroupId.Unset()
}

// GetTriagedTeamNames returns the TriagedTeamNames field value if set, zero value otherwise.
func (o *TeamData) GetTriagedTeamNames() []string {
	if o == nil || IsNil(o.TriagedTeamNames) {
		var ret []string
		return ret
	}
	return o.TriagedTeamNames
}

// GetTriagedTeamNamesOk returns a tuple with the TriagedTeamNames field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TeamData) GetTriagedTeamNamesOk() ([]string, bool) {
	if o == nil || IsNil(o.TriagedTeamNames) {
		return nil, false
	}
	return o.TriagedTeamNames, true
}

// HasTriagedTeamNames returns a boolean if a field has been set.
func (o *TeamData) HasTriagedTeamNames() bool {
	if o != nil && !IsNil(o.TriagedTeamNames) {
		return true
	}

	return false
}

// SetTriagedTeamNames gets a reference to the given []string and assigns it to the TriagedTeamNames field.
func (o *TeamData) SetTriagedTeamNames(v []string) {
	o.TriagedTeamNames = v
}

func (o TeamData) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o TeamData) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	toSerialize["team_name"] = o.TeamName
	toSerialize["jira_project"] = o.JiraProject
	toSerialize["slack_channel_id"] = o.SlackChannelId.Get()
	toSerialize["evergreen_tag_name"] = o.EvergreenTagName
	if o.TriageTeamName.IsSet() {
		toSerialize["triage_team_name"] = o.TriageTeamName.Get()
	}
	if o.SlackGroupId.IsSet() {
		toSerialize["slack_group_id"] = o.SlackGroupId.Get()
	}
	if !IsNil(o.TriagedTeamNames) {
		toSerialize["triaged_team_names"] = o.TriagedTeamNames
	}
	return toSerialize, nil
}

func (o *TeamData) UnmarshalJSON(data []byte) (err error) {
	// This validates that all required properties are included in the JSON object
	// by unmarshalling the object into a generic map with string keys and checking
	// that every required field exists as a key in the generic map.
	requiredProperties := []string{
		"team_name",
		"jira_project",
		"slack_channel_id",
		"evergreen_tag_name",
	}

	allProperties := make(map[string]interface{})

	err = json.Unmarshal(data, &allProperties)

	if err != nil {
		return err;
	}

	for _, requiredProperty := range(requiredProperties) {
		if _, exists := allProperties[requiredProperty]; !exists {
			return fmt.Errorf("no value given for required property %v", requiredProperty)
		}
	}

	varTeamData := _TeamData{}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&varTeamData)

	if err != nil {
		return err
	}

	*o = TeamData(varTeamData)

	return err
}

type NullableTeamData struct {
	value *TeamData
	isSet bool
}

func (v NullableTeamData) Get() *TeamData {
	return v.value
}

func (v *NullableTeamData) Set(val *TeamData) {
	v.value = val
	v.isSet = true
}

func (v NullableTeamData) IsSet() bool {
	return v.isSet
}

func (v *NullableTeamData) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableTeamData(val *TeamData) *NullableTeamData {
	return &NullableTeamData{value: val, isSet: true}
}

func (v NullableTeamData) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableTeamData) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


