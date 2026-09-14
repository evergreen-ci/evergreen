# ValidationError

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Loc** | [**[]LocationInner**](LocationInner.md) |  |
**Msg** | **string** |  |
**Type** | **string** |  |
**Input** | Pointer to **interface{}** |  | [optional]
**Ctx** | Pointer to **map[string]interface{}** |  | [optional]

## Methods

### NewValidationError

`func NewValidationError(loc []LocationInner, msg string, type_ string, ) *ValidationError`

NewValidationError instantiates a new ValidationError object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewValidationErrorWithDefaults

`func NewValidationErrorWithDefaults() *ValidationError`

NewValidationErrorWithDefaults instantiates a new ValidationError object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetLoc

`func (o *ValidationError) GetLoc() []LocationInner`

GetLoc returns the Loc field if non-nil, zero value otherwise.

### GetLocOk

`func (o *ValidationError) GetLocOk() (*[]LocationInner, bool)`

GetLocOk returns a tuple with the Loc field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLoc

`func (o *ValidationError) SetLoc(v []LocationInner)`

SetLoc sets Loc field to given value.


### GetMsg

`func (o *ValidationError) GetMsg() string`

GetMsg returns the Msg field if non-nil, zero value otherwise.

### GetMsgOk

`func (o *ValidationError) GetMsgOk() (*string, bool)`

GetMsgOk returns a tuple with the Msg field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMsg

`func (o *ValidationError) SetMsg(v string)`

SetMsg sets Msg field to given value.


### GetType

`func (o *ValidationError) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *ValidationError) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *ValidationError) SetType(v string)`

SetType sets Type field to given value.


### GetInput

`func (o *ValidationError) GetInput() interface{}`

GetInput returns the Input field if non-nil, zero value otherwise.

### GetInputOk

`func (o *ValidationError) GetInputOk() (*interface{}, bool)`

GetInputOk returns a tuple with the Input field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInput

`func (o *ValidationError) SetInput(v interface{})`

SetInput sets Input field to given value.

### HasInput

`func (o *ValidationError) HasInput() bool`

HasInput returns a boolean if a field has been set.

### SetInputNil

`func (o *ValidationError) SetInputNil(b bool)`

 SetInputNil sets the value for Input to be an explicit nil

### UnsetInput
`func (o *ValidationError) UnsetInput()`

UnsetInput ensures that no value is present for Input, not even an explicit nil
### GetCtx

`func (o *ValidationError) GetCtx() map[string]interface{}`

GetCtx returns the Ctx field if non-nil, zero value otherwise.

### GetCtxOk

`func (o *ValidationError) GetCtxOk() (*map[string]interface{}, bool)`

GetCtxOk returns a tuple with the Ctx field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCtx

`func (o *ValidationError) SetCtx(v map[string]interface{})`

SetCtx sets Ctx field to given value.

### HasCtx

`func (o *ValidationError) HasCtx() bool`

HasCtx returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


