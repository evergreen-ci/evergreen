# TeamDataWithOwner

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**TeamData** | [**TeamData**](TeamData.md) |  | 
**OwningUser** | Pointer to **NullableString** |  | [optional] 

## Methods

### NewTeamDataWithOwner

`func NewTeamDataWithOwner(teamData TeamData, ) *TeamDataWithOwner`

NewTeamDataWithOwner instantiates a new TeamDataWithOwner object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewTeamDataWithOwnerWithDefaults

`func NewTeamDataWithOwnerWithDefaults() *TeamDataWithOwner`

NewTeamDataWithOwnerWithDefaults instantiates a new TeamDataWithOwner object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetTeamData

`func (o *TeamDataWithOwner) GetTeamData() TeamData`

GetTeamData returns the TeamData field if non-nil, zero value otherwise.

### GetTeamDataOk

`func (o *TeamDataWithOwner) GetTeamDataOk() (*TeamData, bool)`

GetTeamDataOk returns a tuple with the TeamData field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTeamData

`func (o *TeamDataWithOwner) SetTeamData(v TeamData)`

SetTeamData sets TeamData field to given value.


### GetOwningUser

`func (o *TeamDataWithOwner) GetOwningUser() string`

GetOwningUser returns the OwningUser field if non-nil, zero value otherwise.

### GetOwningUserOk

`func (o *TeamDataWithOwner) GetOwningUserOk() (*string, bool)`

GetOwningUserOk returns a tuple with the OwningUser field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOwningUser

`func (o *TeamDataWithOwner) SetOwningUser(v string)`

SetOwningUser sets OwningUser field to given value.

### HasOwningUser

`func (o *TeamDataWithOwner) HasOwningUser() bool`

HasOwningUser returns a boolean if a field has been set.

### SetOwningUserNil

`func (o *TeamDataWithOwner) SetOwningUserNil(b bool)`

 SetOwningUserNil sets the value for OwningUser to be an explicit nil

### UnsetOwningUser
`func (o *TeamDataWithOwner) UnsetOwningUser()`

UnsetOwningUser ensures that no value is present for OwningUser, not even an explicit nil

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


