/*
 * HCS API
 *
 * No description provided (generated by Swagger Codegen https://github.com/swagger-api/swagger-codegen)
 *
 * API version: 2.1
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package hcsschema

type StorageQoS struct {
	IopsMaximum int32 `json:"IopsMaximum,omitempty"`

	BandwidthMaximum int32 `json:"BandwidthMaximum,omitempty"`
}
