# AssignmentResult

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**AssignmentType** | [**AssignmentTypeEnum**](AssignmentTypeEnum.md) |  | 
**Messages** | **string** |  | 
**TeamDataWithOwner** | Pointer to [**NullableTeamDataWithOwner**](TeamDataWithOwner.md) |  | [optional] 

## Methods

### NewAssignmentResult

`func NewAssignmentResult(assignmentType AssignmentTypeEnum, messages string, ) *AssignmentResult`

NewAssignmentResult instantiates a new AssignmentResult object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAssignmentResultWithDefaults

`func NewAssignmentResultWithDefaults() *AssignmentResult`

NewAssignmentResultWithDefaults instantiates a new AssignmentResult object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAssignmentType

`func (o *AssignmentResult) GetAssignmentType() AssignmentTypeEnum`

GetAssignmentType returns the AssignmentType field if non-nil, zero value otherwise.

### GetAssignmentTypeOk

`func (o *AssignmentResult) GetAssignmentTypeOk() (*AssignmentTypeEnum, bool)`

GetAssignmentTypeOk returns a tuple with the AssignmentType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAssignmentType

`func (o *AssignmentResult) SetAssignmentType(v AssignmentTypeEnum)`

SetAssignmentType sets AssignmentType field to given value.


### GetMessages

`func (o *AssignmentResult) GetMessages() string`

GetMessages returns the Messages field if non-nil, zero value otherwise.

### GetMessagesOk

`func (o *AssignmentResult) GetMessagesOk() (*string, bool)`

GetMessagesOk returns a tuple with the Messages field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMessages

`func (o *AssignmentResult) SetMessages(v string)`

SetMessages sets Messages field to given value.


### GetTeamDataWithOwner

`func (o *AssignmentResult) GetTeamDataWithOwner() TeamDataWithOwner`

GetTeamDataWithOwner returns the TeamDataWithOwner field if non-nil, zero value otherwise.

### GetTeamDataWithOwnerOk

`func (o *AssignmentResult) GetTeamDataWithOwnerOk() (*TeamDataWithOwner, bool)`

GetTeamDataWithOwnerOk returns a tuple with the TeamDataWithOwner field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTeamDataWithOwner

`func (o *AssignmentResult) SetTeamDataWithOwner(v TeamDataWithOwner)`

SetTeamDataWithOwner sets TeamDataWithOwner field to given value.

### HasTeamDataWithOwner

`func (o *AssignmentResult) HasTeamDataWithOwner() bool`

HasTeamDataWithOwner returns a boolean if a field has been set.

### SetTeamDataWithOwnerNil

`func (o *AssignmentResult) SetTeamDataWithOwnerNil(b bool)`

 SetTeamDataWithOwnerNil sets the value for TeamDataWithOwner to be an explicit nil

### UnsetTeamDataWithOwner
`func (o *AssignmentResult) UnsetTeamDataWithOwner()`

UnsetTeamDataWithOwner ensures that no value is present for TeamDataWithOwner, not even an explicit nil

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


