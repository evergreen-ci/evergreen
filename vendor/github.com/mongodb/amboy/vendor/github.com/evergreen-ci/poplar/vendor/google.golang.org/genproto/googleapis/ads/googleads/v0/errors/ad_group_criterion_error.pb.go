// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v0/errors/ad_group_criterion_error.proto

package errors // import "google.golang.org/genproto/googleapis/ads/googleads/v0/errors"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// Enum describing possible ad group criterion errors.
type AdGroupCriterionErrorEnum_AdGroupCriterionError int32

const (
	// Enum unspecified.
	AdGroupCriterionErrorEnum_UNSPECIFIED AdGroupCriterionErrorEnum_AdGroupCriterionError = 0
	// The received error code is not known in this version.
	AdGroupCriterionErrorEnum_UNKNOWN AdGroupCriterionErrorEnum_AdGroupCriterionError = 1
	// No link found between the AdGroupCriterion and the label.
	AdGroupCriterionErrorEnum_AD_GROUP_CRITERION_LABEL_DOES_NOT_EXIST AdGroupCriterionErrorEnum_AdGroupCriterionError = 2
	// The label has already been attached to the AdGroupCriterion.
	AdGroupCriterionErrorEnum_AD_GROUP_CRITERION_LABEL_ALREADY_EXISTS AdGroupCriterionErrorEnum_AdGroupCriterionError = 3
	// Negative AdGroupCriterion cannot have labels.
	AdGroupCriterionErrorEnum_CANNOT_ADD_LABEL_TO_NEGATIVE_CRITERION AdGroupCriterionErrorEnum_AdGroupCriterionError = 4
	// Too many operations for a single call.
	AdGroupCriterionErrorEnum_TOO_MANY_OPERATIONS AdGroupCriterionErrorEnum_AdGroupCriterionError = 5
	// Negative ad group criteria are not updateable.
	AdGroupCriterionErrorEnum_CANT_UPDATE_NEGATIVE AdGroupCriterionErrorEnum_AdGroupCriterionError = 6
	// Concrete type of criterion (keyword v.s. placement) is required for ADD
	// and SET operations.
	AdGroupCriterionErrorEnum_CONCRETE_TYPE_REQUIRED AdGroupCriterionErrorEnum_AdGroupCriterionError = 7
	// Bid is incompatible with ad group's bidding settings.
	AdGroupCriterionErrorEnum_BID_INCOMPATIBLE_WITH_ADGROUP AdGroupCriterionErrorEnum_AdGroupCriterionError = 8
	// Cannot target and exclude the same criterion at once.
	AdGroupCriterionErrorEnum_CANNOT_TARGET_AND_EXCLUDE AdGroupCriterionErrorEnum_AdGroupCriterionError = 9
	// The URL of a placement is invalid.
	AdGroupCriterionErrorEnum_ILLEGAL_URL AdGroupCriterionErrorEnum_AdGroupCriterionError = 10
	// Keyword text was invalid.
	AdGroupCriterionErrorEnum_INVALID_KEYWORD_TEXT AdGroupCriterionErrorEnum_AdGroupCriterionError = 11
	// Destination URL was invalid.
	AdGroupCriterionErrorEnum_INVALID_DESTINATION_URL AdGroupCriterionErrorEnum_AdGroupCriterionError = 12
	// The destination url must contain at least one tag (e.g. {lpurl})
	AdGroupCriterionErrorEnum_MISSING_DESTINATION_URL_TAG AdGroupCriterionErrorEnum_AdGroupCriterionError = 13
	// Keyword-level cpm bid is not supported
	AdGroupCriterionErrorEnum_KEYWORD_LEVEL_BID_NOT_SUPPORTED_FOR_MANUALCPM AdGroupCriterionErrorEnum_AdGroupCriterionError = 14
	// For example, cannot add a biddable ad group criterion that had been
	// removed.
	AdGroupCriterionErrorEnum_INVALID_USER_STATUS AdGroupCriterionErrorEnum_AdGroupCriterionError = 15
	// Criteria type cannot be targeted for the ad group. Either the account is
	// restricted to keywords only, the criteria type is incompatible with the
	// campaign's bidding strategy, or the criteria type can only be applied to
	// campaigns.
	AdGroupCriterionErrorEnum_CANNOT_ADD_CRITERIA_TYPE AdGroupCriterionErrorEnum_AdGroupCriterionError = 16
	// Criteria type cannot be excluded for the ad group. Refer to the
	// documentation for a specific criterion to check if it is excludable.
	AdGroupCriterionErrorEnum_CANNOT_EXCLUDE_CRITERIA_TYPE AdGroupCriterionErrorEnum_AdGroupCriterionError = 17
	// Partial failure is not supported for shopping campaign mutate operations.
	AdGroupCriterionErrorEnum_CAMPAIGN_TYPE_NOT_COMPATIBLE_WITH_PARTIAL_FAILURE AdGroupCriterionErrorEnum_AdGroupCriterionError = 27
	// Operations in the mutate request changes too many shopping ad groups.
	// Please split requests for multiple shopping ad groups across multiple
	// requests.
	AdGroupCriterionErrorEnum_OPERATIONS_FOR_TOO_MANY_SHOPPING_ADGROUPS AdGroupCriterionErrorEnum_AdGroupCriterionError = 28
	// Not allowed to modify url fields of an ad group criterion if there are
	// duplicate elements for that ad group criterion in the request.
	AdGroupCriterionErrorEnum_CANNOT_MODIFY_URL_FIELDS_WITH_DUPLICATE_ELEMENTS AdGroupCriterionErrorEnum_AdGroupCriterionError = 29
	// Cannot set url fields without also setting final urls.
	AdGroupCriterionErrorEnum_CANNOT_SET_WITHOUT_FINAL_URLS AdGroupCriterionErrorEnum_AdGroupCriterionError = 30
	// Cannot clear final urls if final mobile urls exist.
	AdGroupCriterionErrorEnum_CANNOT_CLEAR_FINAL_URLS_IF_FINAL_MOBILE_URLS_EXIST AdGroupCriterionErrorEnum_AdGroupCriterionError = 31
	// Cannot clear final urls if final app urls exist.
	AdGroupCriterionErrorEnum_CANNOT_CLEAR_FINAL_URLS_IF_FINAL_APP_URLS_EXIST AdGroupCriterionErrorEnum_AdGroupCriterionError = 32
	// Cannot clear final urls if tracking url template exists.
	AdGroupCriterionErrorEnum_CANNOT_CLEAR_FINAL_URLS_IF_TRACKING_URL_TEMPLATE_EXISTS AdGroupCriterionErrorEnum_AdGroupCriterionError = 33
	// Cannot clear final urls if url custom parameters exist.
	AdGroupCriterionErrorEnum_CANNOT_CLEAR_FINAL_URLS_IF_URL_CUSTOM_PARAMETERS_EXIST AdGroupCriterionErrorEnum_AdGroupCriterionError = 34
	// Cannot set both destination url and final urls.
	AdGroupCriterionErrorEnum_CANNOT_SET_BOTH_DESTINATION_URL_AND_FINAL_URLS AdGroupCriterionErrorEnum_AdGroupCriterionError = 35
	// Cannot set both destination url and tracking url template.
	AdGroupCriterionErrorEnum_CANNOT_SET_BOTH_DESTINATION_URL_AND_TRACKING_URL_TEMPLATE AdGroupCriterionErrorEnum_AdGroupCriterionError = 36
	// Final urls are not supported for this criterion type.
	AdGroupCriterionErrorEnum_FINAL_URLS_NOT_SUPPORTED_FOR_CRITERION_TYPE AdGroupCriterionErrorEnum_AdGroupCriterionError = 37
	// Final mobile urls are not supported for this criterion type.
	AdGroupCriterionErrorEnum_FINAL_MOBILE_URLS_NOT_SUPPORTED_FOR_CRITERION_TYPE AdGroupCriterionErrorEnum_AdGroupCriterionError = 38
	// Ad group is invalid due to the listing groups it contains.
	AdGroupCriterionErrorEnum_INVALID_LISTING_GROUP_HIERARCHY AdGroupCriterionErrorEnum_AdGroupCriterionError = 39
	// Listing group unit cannot have children.
	AdGroupCriterionErrorEnum_LISTING_GROUP_UNIT_CANNOT_HAVE_CHILDREN AdGroupCriterionErrorEnum_AdGroupCriterionError = 40
	// Subdivided listing groups must have an "others" case.
	AdGroupCriterionErrorEnum_LISTING_GROUP_SUBDIVISION_REQUIRES_OTHERS_CASE AdGroupCriterionErrorEnum_AdGroupCriterionError = 41
	// Dimension type of listing group must be the same as that of its siblings.
	AdGroupCriterionErrorEnum_LISTING_GROUP_REQUIRES_SAME_DIMENSION_TYPE_AS_SIBLINGS AdGroupCriterionErrorEnum_AdGroupCriterionError = 42
	// Listing group cannot be added to the ad group because it already exists.
	AdGroupCriterionErrorEnum_LISTING_GROUP_ALREADY_EXISTS AdGroupCriterionErrorEnum_AdGroupCriterionError = 43
	// Listing group referenced in the operation was not found in the ad group.
	AdGroupCriterionErrorEnum_LISTING_GROUP_DOES_NOT_EXIST AdGroupCriterionErrorEnum_AdGroupCriterionError = 44
	// Recursive removal failed because listing group subdivision is being
	// created or modified in this request.
	AdGroupCriterionErrorEnum_LISTING_GROUP_CANNOT_BE_REMOVED AdGroupCriterionErrorEnum_AdGroupCriterionError = 45
	// Listing group type is not allowed for specified ad group criterion type.
	AdGroupCriterionErrorEnum_INVALID_LISTING_GROUP_TYPE AdGroupCriterionErrorEnum_AdGroupCriterionError = 46
	// Listing group in an ADD operation specifies a non temporary criterion id.
	AdGroupCriterionErrorEnum_LISTING_GROUP_ADD_MAY_ONLY_USE_TEMP_ID AdGroupCriterionErrorEnum_AdGroupCriterionError = 47
)

var AdGroupCriterionErrorEnum_AdGroupCriterionError_name = map[int32]string{
	0:  "UNSPECIFIED",
	1:  "UNKNOWN",
	2:  "AD_GROUP_CRITERION_LABEL_DOES_NOT_EXIST",
	3:  "AD_GROUP_CRITERION_LABEL_ALREADY_EXISTS",
	4:  "CANNOT_ADD_LABEL_TO_NEGATIVE_CRITERION",
	5:  "TOO_MANY_OPERATIONS",
	6:  "CANT_UPDATE_NEGATIVE",
	7:  "CONCRETE_TYPE_REQUIRED",
	8:  "BID_INCOMPATIBLE_WITH_ADGROUP",
	9:  "CANNOT_TARGET_AND_EXCLUDE",
	10: "ILLEGAL_URL",
	11: "INVALID_KEYWORD_TEXT",
	12: "INVALID_DESTINATION_URL",
	13: "MISSING_DESTINATION_URL_TAG",
	14: "KEYWORD_LEVEL_BID_NOT_SUPPORTED_FOR_MANUALCPM",
	15: "INVALID_USER_STATUS",
	16: "CANNOT_ADD_CRITERIA_TYPE",
	17: "CANNOT_EXCLUDE_CRITERIA_TYPE",
	27: "CAMPAIGN_TYPE_NOT_COMPATIBLE_WITH_PARTIAL_FAILURE",
	28: "OPERATIONS_FOR_TOO_MANY_SHOPPING_ADGROUPS",
	29: "CANNOT_MODIFY_URL_FIELDS_WITH_DUPLICATE_ELEMENTS",
	30: "CANNOT_SET_WITHOUT_FINAL_URLS",
	31: "CANNOT_CLEAR_FINAL_URLS_IF_FINAL_MOBILE_URLS_EXIST",
	32: "CANNOT_CLEAR_FINAL_URLS_IF_FINAL_APP_URLS_EXIST",
	33: "CANNOT_CLEAR_FINAL_URLS_IF_TRACKING_URL_TEMPLATE_EXISTS",
	34: "CANNOT_CLEAR_FINAL_URLS_IF_URL_CUSTOM_PARAMETERS_EXIST",
	35: "CANNOT_SET_BOTH_DESTINATION_URL_AND_FINAL_URLS",
	36: "CANNOT_SET_BOTH_DESTINATION_URL_AND_TRACKING_URL_TEMPLATE",
	37: "FINAL_URLS_NOT_SUPPORTED_FOR_CRITERION_TYPE",
	38: "FINAL_MOBILE_URLS_NOT_SUPPORTED_FOR_CRITERION_TYPE",
	39: "INVALID_LISTING_GROUP_HIERARCHY",
	40: "LISTING_GROUP_UNIT_CANNOT_HAVE_CHILDREN",
	41: "LISTING_GROUP_SUBDIVISION_REQUIRES_OTHERS_CASE",
	42: "LISTING_GROUP_REQUIRES_SAME_DIMENSION_TYPE_AS_SIBLINGS",
	43: "LISTING_GROUP_ALREADY_EXISTS",
	44: "LISTING_GROUP_DOES_NOT_EXIST",
	45: "LISTING_GROUP_CANNOT_BE_REMOVED",
	46: "INVALID_LISTING_GROUP_TYPE",
	47: "LISTING_GROUP_ADD_MAY_ONLY_USE_TEMP_ID",
}
var AdGroupCriterionErrorEnum_AdGroupCriterionError_value = map[string]int32{
	"UNSPECIFIED": 0,
	"UNKNOWN":     1,
	"AD_GROUP_CRITERION_LABEL_DOES_NOT_EXIST":                   2,
	"AD_GROUP_CRITERION_LABEL_ALREADY_EXISTS":                   3,
	"CANNOT_ADD_LABEL_TO_NEGATIVE_CRITERION":                    4,
	"TOO_MANY_OPERATIONS":                                       5,
	"CANT_UPDATE_NEGATIVE":                                      6,
	"CONCRETE_TYPE_REQUIRED":                                    7,
	"BID_INCOMPATIBLE_WITH_ADGROUP":                             8,
	"CANNOT_TARGET_AND_EXCLUDE":                                 9,
	"ILLEGAL_URL":                                               10,
	"INVALID_KEYWORD_TEXT":                                      11,
	"INVALID_DESTINATION_URL":                                   12,
	"MISSING_DESTINATION_URL_TAG":                               13,
	"KEYWORD_LEVEL_BID_NOT_SUPPORTED_FOR_MANUALCPM":             14,
	"INVALID_USER_STATUS":                                       15,
	"CANNOT_ADD_CRITERIA_TYPE":                                  16,
	"CANNOT_EXCLUDE_CRITERIA_TYPE":                              17,
	"CAMPAIGN_TYPE_NOT_COMPATIBLE_WITH_PARTIAL_FAILURE":         27,
	"OPERATIONS_FOR_TOO_MANY_SHOPPING_ADGROUPS":                 28,
	"CANNOT_MODIFY_URL_FIELDS_WITH_DUPLICATE_ELEMENTS":          29,
	"CANNOT_SET_WITHOUT_FINAL_URLS":                             30,
	"CANNOT_CLEAR_FINAL_URLS_IF_FINAL_MOBILE_URLS_EXIST":        31,
	"CANNOT_CLEAR_FINAL_URLS_IF_FINAL_APP_URLS_EXIST":           32,
	"CANNOT_CLEAR_FINAL_URLS_IF_TRACKING_URL_TEMPLATE_EXISTS":   33,
	"CANNOT_CLEAR_FINAL_URLS_IF_URL_CUSTOM_PARAMETERS_EXIST":    34,
	"CANNOT_SET_BOTH_DESTINATION_URL_AND_FINAL_URLS":            35,
	"CANNOT_SET_BOTH_DESTINATION_URL_AND_TRACKING_URL_TEMPLATE": 36,
	"FINAL_URLS_NOT_SUPPORTED_FOR_CRITERION_TYPE":               37,
	"FINAL_MOBILE_URLS_NOT_SUPPORTED_FOR_CRITERION_TYPE":        38,
	"INVALID_LISTING_GROUP_HIERARCHY":                           39,
	"LISTING_GROUP_UNIT_CANNOT_HAVE_CHILDREN":                   40,
	"LISTING_GROUP_SUBDIVISION_REQUIRES_OTHERS_CASE":            41,
	"LISTING_GROUP_REQUIRES_SAME_DIMENSION_TYPE_AS_SIBLINGS":    42,
	"LISTING_GROUP_ALREADY_EXISTS":                              43,
	"LISTING_GROUP_DOES_NOT_EXIST":                              44,
	"LISTING_GROUP_CANNOT_BE_REMOVED":                           45,
	"INVALID_LISTING_GROUP_TYPE":                                46,
	"LISTING_GROUP_ADD_MAY_ONLY_USE_TEMP_ID":                    47,
}

func (x AdGroupCriterionErrorEnum_AdGroupCriterionError) String() string {
	return proto.EnumName(AdGroupCriterionErrorEnum_AdGroupCriterionError_name, int32(x))
}
func (AdGroupCriterionErrorEnum_AdGroupCriterionError) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_ad_group_criterion_error_be396cc418d13422, []int{0, 0}
}

// Container for enum describing possible ad group criterion errors.
type AdGroupCriterionErrorEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *AdGroupCriterionErrorEnum) Reset()         { *m = AdGroupCriterionErrorEnum{} }
func (m *AdGroupCriterionErrorEnum) String() string { return proto.CompactTextString(m) }
func (*AdGroupCriterionErrorEnum) ProtoMessage()    {}
func (*AdGroupCriterionErrorEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_ad_group_criterion_error_be396cc418d13422, []int{0}
}
func (m *AdGroupCriterionErrorEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_AdGroupCriterionErrorEnum.Unmarshal(m, b)
}
func (m *AdGroupCriterionErrorEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_AdGroupCriterionErrorEnum.Marshal(b, m, deterministic)
}
func (dst *AdGroupCriterionErrorEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_AdGroupCriterionErrorEnum.Merge(dst, src)
}
func (m *AdGroupCriterionErrorEnum) XXX_Size() int {
	return xxx_messageInfo_AdGroupCriterionErrorEnum.Size(m)
}
func (m *AdGroupCriterionErrorEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_AdGroupCriterionErrorEnum.DiscardUnknown(m)
}

var xxx_messageInfo_AdGroupCriterionErrorEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*AdGroupCriterionErrorEnum)(nil), "google.ads.googleads.v0.errors.AdGroupCriterionErrorEnum")
	proto.RegisterEnum("google.ads.googleads.v0.errors.AdGroupCriterionErrorEnum_AdGroupCriterionError", AdGroupCriterionErrorEnum_AdGroupCriterionError_name, AdGroupCriterionErrorEnum_AdGroupCriterionError_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v0/errors/ad_group_criterion_error.proto", fileDescriptor_ad_group_criterion_error_be396cc418d13422)
}

var fileDescriptor_ad_group_criterion_error_be396cc418d13422 = []byte{
	// 958 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x55, 0xdd, 0x72, 0x1b, 0x35,
	0x14, 0x26, 0x29, 0xb4, 0xa0, 0x04, 0x2a, 0xc4, 0x4f, 0xdb, 0xfc, 0xb6, 0x29, 0xb4, 0xb4, 0x21,
	0x6b, 0xb7, 0x85, 0x32, 0xb8, 0xd3, 0x8b, 0xe3, 0xd5, 0xf1, 0x5a, 0x13, 0xad, 0xb4, 0x48, 0x5a,
	0x27, 0x66, 0x32, 0xa3, 0x09, 0x75, 0xc6, 0x93, 0x99, 0x36, 0xce, 0xd8, 0x6d, 0x1f, 0x88, 0x4b,
	0xde, 0x80, 0x57, 0xe0, 0x05, 0x78, 0x07, 0xee, 0xb9, 0x67, 0xb4, 0xbb, 0x76, 0x6c, 0x27, 0xa4,
	0xbd, 0x8a, 0xe2, 0xf3, 0x7d, 0xd2, 0x77, 0xbe, 0x73, 0xf6, 0x1c, 0xf2, 0xbc, 0x3f, 0x18, 0xf4,
	0x5f, 0x1e, 0xd5, 0x0e, 0x7b, 0xa3, 0x5a, 0x79, 0x0c, 0xa7, 0xb7, 0xf5, 0xda, 0xd1, 0x70, 0x38,
	0x18, 0x8e, 0x6a, 0x87, 0x3d, 0xdf, 0x1f, 0x0e, 0xde, 0x9c, 0xfa, 0x17, 0xc3, 0xe3, 0xd7, 0x47,
	0xc3, 0xe3, 0xc1, 0x89, 0x2f, 0x22, 0xd1, 0xe9, 0x70, 0xf0, 0x7a, 0xc0, 0x36, 0x4a, 0x4e, 0x74,
	0xd8, 0x1b, 0x45, 0x13, 0x7a, 0xf4, 0xb6, 0x1e, 0x95, 0xf4, 0xad, 0xbf, 0x97, 0xc9, 0x2d, 0xe8,
	0x25, 0xe1, 0x86, 0x78, 0x7c, 0x01, 0x86, 0x10, 0x9e, 0xbc, 0x79, 0xb5, 0xf5, 0xe7, 0x32, 0xf9,
	0xea, 0xc2, 0x28, 0xbb, 0x4e, 0x96, 0x72, 0x65, 0x33, 0x8c, 0x45, 0x4b, 0x20, 0xa7, 0x1f, 0xb0,
	0x25, 0x72, 0x2d, 0x57, 0xbb, 0x4a, 0xef, 0x29, 0xba, 0xc0, 0xb6, 0xc9, 0x7d, 0xe0, 0x3e, 0x31,
	0x3a, 0xcf, 0x7c, 0x6c, 0x84, 0x43, 0x23, 0xb4, 0xf2, 0x12, 0x9a, 0x28, 0x3d, 0xd7, 0x68, 0xbd,
	0xd2, 0xce, 0xe3, 0xbe, 0xb0, 0x8e, 0x2e, 0x5e, 0x0a, 0x06, 0x69, 0x10, 0x78, 0xb7, 0xc4, 0x5a,
	0x7a, 0x85, 0x3d, 0x24, 0xf7, 0x62, 0x50, 0x81, 0x0e, 0x9c, 0x57, 0x20, 0xa7, 0xbd, 0xc2, 0x04,
	0x9c, 0xe8, 0xe0, 0xd9, 0x05, 0xf4, 0x43, 0x76, 0x83, 0x7c, 0xe1, 0xb4, 0xf6, 0x29, 0xa8, 0xae,
	0xd7, 0x19, 0x1a, 0x70, 0x42, 0x2b, 0x4b, 0x3f, 0x62, 0x37, 0xc9, 0x97, 0x31, 0x28, 0xe7, 0xf3,
	0x8c, 0x83, 0xc3, 0x09, 0x99, 0x5e, 0x65, 0x2b, 0xe4, 0xeb, 0x58, 0xab, 0xd8, 0xa0, 0x43, 0xef,
	0xba, 0x19, 0x7a, 0x83, 0xbf, 0xe4, 0xc2, 0x20, 0xa7, 0xd7, 0xd8, 0x1d, 0xb2, 0xde, 0x14, 0xdc,
	0x0b, 0x15, 0xeb, 0x34, 0x03, 0x27, 0x9a, 0x12, 0xfd, 0x9e, 0x70, 0x6d, 0x0f, 0xbc, 0x10, 0x4f,
	0x3f, 0x66, 0xeb, 0xe4, 0x56, 0xa5, 0xce, 0x81, 0x49, 0xd0, 0x79, 0x50, 0xdc, 0xe3, 0x7e, 0x2c,
	0x73, 0x8e, 0xf4, 0x93, 0x60, 0x9a, 0x90, 0x12, 0x13, 0x90, 0x3e, 0x37, 0x92, 0x92, 0x20, 0x44,
	0xa8, 0x0e, 0x48, 0xc1, 0xfd, 0x2e, 0x76, 0xf7, 0xb4, 0xe1, 0xde, 0xe1, 0xbe, 0xa3, 0x4b, 0x6c,
	0x95, 0xdc, 0x18, 0x47, 0x38, 0x5a, 0x27, 0x54, 0x21, 0xbe, 0xa0, 0x2d, 0xb3, 0x4d, 0xb2, 0x9a,
	0x0a, 0x6b, 0x85, 0x4a, 0xe6, 0x83, 0xde, 0x41, 0x42, 0x3f, 0x65, 0x8f, 0xc8, 0xce, 0xf8, 0x3e,
	0x89, 0x1d, 0x94, 0x3e, 0x08, 0x0f, 0xb2, 0x6c, 0x9e, 0x65, 0xda, 0x38, 0xe4, 0xbe, 0xa5, 0x4d,
	0x70, 0x26, 0x07, 0x19, 0x67, 0x29, 0xfd, 0x2c, 0x98, 0x35, 0x7e, 0x30, 0xb7, 0x68, 0xbc, 0x75,
	0xe0, 0x72, 0x4b, 0xaf, 0xb3, 0x35, 0x72, 0x73, 0xca, 0xf1, 0xca, 0x5f, 0x28, 0xdc, 0xa1, 0x94,
	0xdd, 0x26, 0x6b, 0x55, 0xb4, 0x4a, 0x73, 0x0e, 0xf1, 0x39, 0xfb, 0x91, 0x3c, 0x8a, 0x21, 0xcd,
	0x40, 0x24, 0xaa, 0xb4, 0x34, 0x80, 0xe7, 0x2d, 0xcc, 0xc0, 0x38, 0x01, 0xd2, 0xb7, 0x40, 0xc8,
	0xdc, 0x20, 0x5d, 0x65, 0x3b, 0xe4, 0xc1, 0x59, 0xcd, 0x0a, 0xb5, 0x93, 0x5a, 0xda, 0xb6, 0xce,
	0xb2, 0x90, 0x7c, 0x65, 0xbc, 0xa5, 0x6b, 0xec, 0x07, 0x52, 0xaf, 0x74, 0xa4, 0x9a, 0x8b, 0x56,
	0xb7, 0x30, 0xa3, 0x25, 0x50, 0x72, 0x5b, 0xbe, 0xc0, 0xf3, 0x4c, 0x8a, 0x38, 0x94, 0x1b, 0x25,
	0xa6, 0xa8, 0x9c, 0xa5, 0xeb, 0xa1, 0xa4, 0x15, 0xcb, 0xa2, 0x2b, 0x70, 0x3a, 0x77, 0xbe, 0x25,
	0x54, 0x59, 0x21, 0x4b, 0x37, 0xd8, 0x53, 0xf2, 0xb8, 0x82, 0xc4, 0x12, 0xc1, 0x4c, 0x05, 0xbd,
	0x68, 0x55, 0xff, 0xa5, 0xba, 0x29, 0x24, 0x96, 0x3f, 0x96, 0x5d, 0xbd, 0xc9, 0x9e, 0x90, 0xda,
	0x3b, 0x79, 0x90, 0x65, 0xd3, 0xa4, 0xdb, 0xec, 0x19, 0xf9, 0xe9, 0x12, 0x92, 0x33, 0x10, 0xef,
	0x86, 0xbc, 0x8b, 0x42, 0x63, 0x9a, 0xc9, 0x22, 0x9d, 0xf2, 0xd3, 0xb8, 0xc3, 0x1a, 0xe4, 0xe9,
	0x25, 0xe4, 0xc0, 0x89, 0x73, 0xeb, 0x74, 0x1a, 0xcc, 0x86, 0x14, 0x1d, 0x9a, 0xf1, 0xc3, 0x5b,
	0xec, 0x31, 0x89, 0xa6, 0x8c, 0x68, 0xea, 0x60, 0xd8, 0x5c, 0x67, 0x85, 0x56, 0x9e, 0x72, 0xe6,
	0x2e, 0x7b, 0x4e, 0x7e, 0x7e, 0x1f, 0xce, 0x85, 0xaa, 0xe9, 0x37, 0xac, 0x46, 0xb6, 0xa7, 0x14,
	0x9e, 0x6f, 0xce, 0xb3, 0x51, 0x50, 0x34, 0xd2, 0xb7, 0xa1, 0x12, 0xe7, 0xed, 0x7e, 0x27, 0xef,
	0x1e, 0xbb, 0x4b, 0x36, 0xc7, 0x9d, 0x2d, 0x45, 0xd0, 0x97, 0x54, 0xc3, 0xa6, 0x2d, 0xd0, 0x80,
	0x89, 0xdb, 0x5d, 0x7a, 0x3f, 0x0c, 0xa1, 0xd9, 0x60, 0xae, 0x84, 0xf3, 0x55, 0x7e, 0x6d, 0x08,
	0x93, 0xa5, 0x2d, 0x24, 0x37, 0xa8, 0xe8, 0x77, 0xc1, 0xad, 0x59, 0xb0, 0xcd, 0x9b, 0x5c, 0x74,
	0x84, 0x0d, 0xaf, 0x56, 0x13, 0xc3, 0x7a, 0xed, 0xda, 0xc1, 0xe2, 0x18, 0x2c, 0xd2, 0x07, 0xa1,
	0x3a, 0xb3, 0x9c, 0x09, 0xce, 0x42, 0x8a, 0x9e, 0x8b, 0x14, 0x95, 0x1d, 0xab, 0xf6, 0x60, 0xbd,
	0x15, 0x4d, 0x29, 0x54, 0x62, 0xe9, 0xc3, 0xf0, 0x91, 0xcd, 0x72, 0xe7, 0xc6, 0xe2, 0xf6, 0x79,
	0xc4, 0xdc, 0x94, 0xfd, 0x3e, 0xb8, 0x30, 0x8b, 0xa8, 0x72, 0x6b, 0x86, 0x19, 0x97, 0xea, 0x0e,
	0x72, 0xba, 0xc3, 0x36, 0xc8, 0xca, 0xc5, 0x56, 0x15, 0x56, 0x46, 0x61, 0xfa, 0xce, 0x09, 0xe1,
	0xdc, 0xa7, 0xd0, 0xf5, 0x5a, 0xc9, 0x6e, 0x98, 0x1b, 0x45, 0x81, 0xbd, 0xe0, 0xb4, 0xd6, 0xfc,
	0x77, 0x81, 0x6c, 0xbd, 0x18, 0xbc, 0x8a, 0x2e, 0x5f, 0x40, 0xcd, 0x95, 0x0b, 0xf7, 0x4b, 0x16,
	0x96, 0x57, 0xb6, 0xf0, 0x2b, 0xaf, 0xd8, 0xfd, 0xc1, 0xcb, 0xc3, 0x93, 0x7e, 0x34, 0x18, 0xf6,
	0x6b, 0xfd, 0xa3, 0x93, 0x62, 0xb5, 0x8d, 0xb7, 0xe1, 0xe9, 0xf1, 0xe8, 0xff, 0x96, 0xe3, 0xb3,
	0xf2, 0xcf, 0xef, 0x8b, 0x57, 0x12, 0x80, 0x3f, 0x16, 0x37, 0x92, 0xf2, 0x32, 0xe8, 0x8d, 0xa2,
	0xf2, 0x18, 0x4e, 0x9d, 0x7a, 0x54, 0x3c, 0x39, 0xfa, 0x6b, 0x0c, 0x38, 0x80, 0xde, 0xe8, 0x60,
	0x02, 0x38, 0xe8, 0xd4, 0x0f, 0x4a, 0xc0, 0x3f, 0x8b, 0x5b, 0xe5, 0xaf, 0x8d, 0x06, 0xf4, 0x46,
	0x8d, 0xc6, 0x04, 0xd2, 0x68, 0x74, 0xea, 0x8d, 0x46, 0x09, 0xfa, 0xed, 0x6a, 0xa1, 0xee, 0xc9,
	0x7f, 0x01, 0x00, 0x00, 0xff, 0xff, 0x2b, 0x22, 0x72, 0xb7, 0xb9, 0x07, 0x00, 0x00,
}
