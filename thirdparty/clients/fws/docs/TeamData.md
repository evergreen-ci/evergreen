# TeamData

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**TeamName** | **string** |  | 
**JiraProject** | **string** |  | 
**SlackChannelId** | **NullableString** |  | 
**EvergreenTagName** | **string** |  | 
**TriageTeamName** | Pointer to **NullableString** |  | [optional] 
**SlackGroupId** | Pointer to **NullableString** |  | [optional] 
**TriagedTeamNames** | Pointer to **[]string** |  | [optional] 
**CodeOwners** | Pointer to **[]string** |  | [optional] 

## Methods

### NewTeamData

`func NewTeamData(teamName string, jiraProject string, slackChannelId NullableString, evergreenTagName string, ) *TeamData`

NewTeamData instantiates a new TeamData object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewTeamDataWithDefaults

`func NewTeamDataWithDefaults() *TeamData`

NewTeamDataWithDefaults instantiates a new TeamData object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetTeamName

`func (o *TeamData) GetTeamName() string`

GetTeamName returns the TeamName field if non-nil, zero value otherwise.

### GetTeamNameOk

`func (o *TeamData) GetTeamNameOk() (*string, bool)`

GetTeamNameOk returns a tuple with the TeamName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTeamName

`func (o *TeamData) SetTeamName(v string)`

SetTeamName sets TeamName field to given value.


### GetJiraProject

`func (o *TeamData) GetJiraProject() string`

GetJiraProject returns the JiraProject field if non-nil, zero value otherwise.

### GetJiraProjectOk

`func (o *TeamData) GetJiraProjectOk() (*string, bool)`

GetJiraProjectOk returns a tuple with the JiraProject field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetJiraProject

`func (o *TeamData) SetJiraProject(v string)`

SetJiraProject sets JiraProject field to given value.


### GetSlackChannelId

`func (o *TeamData) GetSlackChannelId() string`

GetSlackChannelId returns the SlackChannelId field if non-nil, zero value otherwise.

### GetSlackChannelIdOk

`func (o *TeamData) GetSlackChannelIdOk() (*string, bool)`

GetSlackChannelIdOk returns a tuple with the SlackChannelId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSlackChannelId

`func (o *TeamData) SetSlackChannelId(v string)`

SetSlackChannelId sets SlackChannelId field to given value.


### SetSlackChannelIdNil

`func (o *TeamData) SetSlackChannelIdNil(b bool)`

 SetSlackChannelIdNil sets the value for SlackChannelId to be an explicit nil

### UnsetSlackChannelId
`func (o *TeamData) UnsetSlackChannelId()`

UnsetSlackChannelId ensures that no value is present for SlackChannelId, not even an explicit nil
### GetEvergreenTagName

`func (o *TeamData) GetEvergreenTagName() string`

GetEvergreenTagName returns the EvergreenTagName field if non-nil, zero value otherwise.

### GetEvergreenTagNameOk

`func (o *TeamData) GetEvergreenTagNameOk() (*string, bool)`

GetEvergreenTagNameOk returns a tuple with the EvergreenTagName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEvergreenTagName

`func (o *TeamData) SetEvergreenTagName(v string)`

SetEvergreenTagName sets EvergreenTagName field to given value.


### GetTriageTeamName

`func (o *TeamData) GetTriageTeamName() string`

GetTriageTeamName returns the TriageTeamName field if non-nil, zero value otherwise.

### GetTriageTeamNameOk

`func (o *TeamData) GetTriageTeamNameOk() (*string, bool)`

GetTriageTeamNameOk returns a tuple with the TriageTeamName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTriageTeamName

`func (o *TeamData) SetTriageTeamName(v string)`

SetTriageTeamName sets TriageTeamName field to given value.

### HasTriageTeamName

`func (o *TeamData) HasTriageTeamName() bool`

HasTriageTeamName returns a boolean if a field has been set.

### SetTriageTeamNameNil

`func (o *TeamData) SetTriageTeamNameNil(b bool)`

 SetTriageTeamNameNil sets the value for TriageTeamName to be an explicit nil

### UnsetTriageTeamName
`func (o *TeamData) UnsetTriageTeamName()`

UnsetTriageTeamName ensures that no value is present for TriageTeamName, not even an explicit nil
### GetSlackGroupId

`func (o *TeamData) GetSlackGroupId() string`

GetSlackGroupId returns the SlackGroupId field if non-nil, zero value otherwise.

### GetSlackGroupIdOk

`func (o *TeamData) GetSlackGroupIdOk() (*string, bool)`

GetSlackGroupIdOk returns a tuple with the SlackGroupId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSlackGroupId

`func (o *TeamData) SetSlackGroupId(v string)`

SetSlackGroupId sets SlackGroupId field to given value.

### HasSlackGroupId

`func (o *TeamData) HasSlackGroupId() bool`

HasSlackGroupId returns a boolean if a field has been set.

### SetSlackGroupIdNil

`func (o *TeamData) SetSlackGroupIdNil(b bool)`

 SetSlackGroupIdNil sets the value for SlackGroupId to be an explicit nil

### UnsetSlackGroupId
`func (o *TeamData) UnsetSlackGroupId()`

UnsetSlackGroupId ensures that no value is present for SlackGroupId, not even an explicit nil
### GetTriagedTeamNames

`func (o *TeamData) GetTriagedTeamNames() []string`

GetTriagedTeamNames returns the TriagedTeamNames field if non-nil, zero value otherwise.

### GetTriagedTeamNamesOk

`func (o *TeamData) GetTriagedTeamNamesOk() (*[]string, bool)`

GetTriagedTeamNamesOk returns a tuple with the TriagedTeamNames field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTriagedTeamNames

`func (o *TeamData) SetTriagedTeamNames(v []string)`

SetTriagedTeamNames sets TriagedTeamNames field to given value.

### HasTriagedTeamNames

`func (o *TeamData) HasTriagedTeamNames() bool`

HasTriagedTeamNames returns a boolean if a field has been set.

### GetCodeOwners

`func (o *TeamData) GetCodeOwners() []string`

GetCodeOwners returns the CodeOwners field if non-nil, zero value otherwise.

### GetCodeOwnersOk

`func (o *TeamData) GetCodeOwnersOk() (*[]string, bool)`

GetCodeOwnersOk returns a tuple with the CodeOwners field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCodeOwners

`func (o *TeamData) SetCodeOwners(v []string)`

SetCodeOwners sets CodeOwners field to given value.

### HasCodeOwners

`func (o *TeamData) HasCodeOwners() bool`

HasCodeOwners returns a boolean if a field has been set.

### SetCodeOwnersNil

`func (o *TeamData) SetCodeOwnersNil(b bool)`

 SetCodeOwnersNil sets the value for CodeOwners to be an explicit nil

### UnsetCodeOwners
`func (o *TeamData) UnsetCodeOwners()`

UnsetCodeOwners ensures that no value is present for CodeOwners, not even an explicit nil

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


