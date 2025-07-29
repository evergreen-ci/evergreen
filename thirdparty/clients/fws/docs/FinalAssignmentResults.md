# FinalAssignmentResults

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**AllAssignmentResults** | [**[]AssignmentResult**](AssignmentResult.md) |  | 
**AllMessages** | **string** |  | 
**SelectedAssignment** | [**AssignmentResult**](AssignmentResult.md) |  | 

## Methods

### NewFinalAssignmentResults

`func NewFinalAssignmentResults(allAssignmentResults []AssignmentResult, allMessages string, selectedAssignment AssignmentResult, ) *FinalAssignmentResults`

NewFinalAssignmentResults instantiates a new FinalAssignmentResults object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewFinalAssignmentResultsWithDefaults

`func NewFinalAssignmentResultsWithDefaults() *FinalAssignmentResults`

NewFinalAssignmentResultsWithDefaults instantiates a new FinalAssignmentResults object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAllAssignmentResults

`func (o *FinalAssignmentResults) GetAllAssignmentResults() []AssignmentResult`

GetAllAssignmentResults returns the AllAssignmentResults field if non-nil, zero value otherwise.

### GetAllAssignmentResultsOk

`func (o *FinalAssignmentResults) GetAllAssignmentResultsOk() (*[]AssignmentResult, bool)`

GetAllAssignmentResultsOk returns a tuple with the AllAssignmentResults field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAllAssignmentResults

`func (o *FinalAssignmentResults) SetAllAssignmentResults(v []AssignmentResult)`

SetAllAssignmentResults sets AllAssignmentResults field to given value.


### GetAllMessages

`func (o *FinalAssignmentResults) GetAllMessages() string`

GetAllMessages returns the AllMessages field if non-nil, zero value otherwise.

### GetAllMessagesOk

`func (o *FinalAssignmentResults) GetAllMessagesOk() (*string, bool)`

GetAllMessagesOk returns a tuple with the AllMessages field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAllMessages

`func (o *FinalAssignmentResults) SetAllMessages(v string)`

SetAllMessages sets AllMessages field to given value.


### GetSelectedAssignment

`func (o *FinalAssignmentResults) GetSelectedAssignment() AssignmentResult`

GetSelectedAssignment returns the SelectedAssignment field if non-nil, zero value otherwise.

### GetSelectedAssignmentOk

`func (o *FinalAssignmentResults) GetSelectedAssignmentOk() (*AssignmentResult, bool)`

GetSelectedAssignmentOk returns a tuple with the SelectedAssignment field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSelectedAssignment

`func (o *FinalAssignmentResults) SetSelectedAssignment(v AssignmentResult)`

SetSelectedAssignment sets SelectedAssignment field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


