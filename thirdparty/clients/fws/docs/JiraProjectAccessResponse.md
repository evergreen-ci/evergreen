# JiraProjectAccessResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Accessible** | **bool** |  |
**Reason** | Pointer to **NullableString** |  | [optional]

## Methods

### NewJiraProjectAccessResponse

`func NewJiraProjectAccessResponse(accessible bool, ) *JiraProjectAccessResponse`

NewJiraProjectAccessResponse instantiates a new JiraProjectAccessResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewJiraProjectAccessResponseWithDefaults

`func NewJiraProjectAccessResponseWithDefaults() *JiraProjectAccessResponse`

NewJiraProjectAccessResponseWithDefaults instantiates a new JiraProjectAccessResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAccessible

`func (o *JiraProjectAccessResponse) GetAccessible() bool`

GetAccessible returns the Accessible field if non-nil, zero value otherwise.

### GetAccessibleOk

`func (o *JiraProjectAccessResponse) GetAccessibleOk() (*bool, bool)`

GetAccessibleOk returns a tuple with the Accessible field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAccessible

`func (o *JiraProjectAccessResponse) SetAccessible(v bool)`

SetAccessible sets Accessible field to given value.


### GetReason

`func (o *JiraProjectAccessResponse) GetReason() string`

GetReason returns the Reason field if non-nil, zero value otherwise.

### GetReasonOk

`func (o *JiraProjectAccessResponse) GetReasonOk() (*string, bool)`

GetReasonOk returns a tuple with the Reason field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReason

`func (o *JiraProjectAccessResponse) SetReason(v string)`

SetReason sets Reason field to given value.

### HasReason

`func (o *JiraProjectAccessResponse) HasReason() bool`

HasReason returns a boolean if a field has been set.

### SetReasonNil

`func (o *JiraProjectAccessResponse) SetReasonNil(b bool)`

 SetReasonNil sets the value for Reason to be an explicit nil

### UnsetReason
`func (o *JiraProjectAccessResponse) UnsetReason()`

UnsetReason ensures that no value is present for Reason, not even an explicit nil

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


