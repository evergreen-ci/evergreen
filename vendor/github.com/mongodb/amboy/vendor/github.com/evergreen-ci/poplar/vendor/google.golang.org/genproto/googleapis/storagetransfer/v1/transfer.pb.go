// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/storagetransfer/v1/transfer.proto

package storagetransfer // import "google.golang.org/genproto/googleapis/storagetransfer/v1"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import empty "github.com/golang/protobuf/ptypes/empty"
import _ "google.golang.org/genproto/googleapis/api/annotations"
import field_mask "google.golang.org/genproto/protobuf/field_mask"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// Request passed to GetGoogleServiceAccount.
type GetGoogleServiceAccountRequest struct {
	// The ID of the Google Cloud Platform Console project that the Google service
	// account is associated with.
	// Required.
	ProjectId            string   `protobuf:"bytes,1,opt,name=project_id,json=projectId,proto3" json:"project_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetGoogleServiceAccountRequest) Reset()         { *m = GetGoogleServiceAccountRequest{} }
func (m *GetGoogleServiceAccountRequest) String() string { return proto.CompactTextString(m) }
func (*GetGoogleServiceAccountRequest) ProtoMessage()    {}
func (*GetGoogleServiceAccountRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_transfer_fe1aac113c6727f1, []int{0}
}
func (m *GetGoogleServiceAccountRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetGoogleServiceAccountRequest.Unmarshal(m, b)
}
func (m *GetGoogleServiceAccountRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetGoogleServiceAccountRequest.Marshal(b, m, deterministic)
}
func (dst *GetGoogleServiceAccountRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetGoogleServiceAccountRequest.Merge(dst, src)
}
func (m *GetGoogleServiceAccountRequest) XXX_Size() int {
	return xxx_messageInfo_GetGoogleServiceAccountRequest.Size(m)
}
func (m *GetGoogleServiceAccountRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetGoogleServiceAccountRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetGoogleServiceAccountRequest proto.InternalMessageInfo

func (m *GetGoogleServiceAccountRequest) GetProjectId() string {
	if m != nil {
		return m.ProjectId
	}
	return ""
}

// Request passed to CreateTransferJob.
type CreateTransferJobRequest struct {
	// The job to create.
	// Required.
	TransferJob          *TransferJob `protobuf:"bytes,1,opt,name=transfer_job,json=transferJob,proto3" json:"transfer_job,omitempty"`
	XXX_NoUnkeyedLiteral struct{}     `json:"-"`
	XXX_unrecognized     []byte       `json:"-"`
	XXX_sizecache        int32        `json:"-"`
}

func (m *CreateTransferJobRequest) Reset()         { *m = CreateTransferJobRequest{} }
func (m *CreateTransferJobRequest) String() string { return proto.CompactTextString(m) }
func (*CreateTransferJobRequest) ProtoMessage()    {}
func (*CreateTransferJobRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_transfer_fe1aac113c6727f1, []int{1}
}
func (m *CreateTransferJobRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_CreateTransferJobRequest.Unmarshal(m, b)
}
func (m *CreateTransferJobRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_CreateTransferJobRequest.Marshal(b, m, deterministic)
}
func (dst *CreateTransferJobRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_CreateTransferJobRequest.Merge(dst, src)
}
func (m *CreateTransferJobRequest) XXX_Size() int {
	return xxx_messageInfo_CreateTransferJobRequest.Size(m)
}
func (m *CreateTransferJobRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_CreateTransferJobRequest.DiscardUnknown(m)
}

var xxx_messageInfo_CreateTransferJobRequest proto.InternalMessageInfo

func (m *CreateTransferJobRequest) GetTransferJob() *TransferJob {
	if m != nil {
		return m.TransferJob
	}
	return nil
}

// Request passed to UpdateTransferJob.
type UpdateTransferJobRequest struct {
	// The name of job to update.
	// Required.
	JobName string `protobuf:"bytes,1,opt,name=job_name,json=jobName,proto3" json:"job_name,omitempty"`
	// The ID of the Google Cloud Platform Console project that owns the job.
	// Required.
	ProjectId string `protobuf:"bytes,2,opt,name=project_id,json=projectId,proto3" json:"project_id,omitempty"`
	// The job to update. `transferJob` is expected to specify only three fields:
	// `description`, `transferSpec`, and `status`.  An UpdateTransferJobRequest
	// that specifies other fields will be rejected with an error
	// `INVALID_ARGUMENT`.
	// Required.
	TransferJob *TransferJob `protobuf:"bytes,3,opt,name=transfer_job,json=transferJob,proto3" json:"transfer_job,omitempty"`
	// The field mask of the fields in `transferJob` that are to be updated in
	// this request.  Fields in `transferJob` that can be updated are:
	// `description`, `transferSpec`, and `status`.  To update the `transferSpec`
	// of the job, a complete transfer specification has to be provided. An
	// incomplete specification which misses any required fields will be rejected
	// with the error `INVALID_ARGUMENT`.
	UpdateTransferJobFieldMask *field_mask.FieldMask `protobuf:"bytes,4,opt,name=update_transfer_job_field_mask,json=updateTransferJobFieldMask,proto3" json:"update_transfer_job_field_mask,omitempty"`
	XXX_NoUnkeyedLiteral       struct{}              `json:"-"`
	XXX_unrecognized           []byte                `json:"-"`
	XXX_sizecache              int32                 `json:"-"`
}

func (m *UpdateTransferJobRequest) Reset()         { *m = UpdateTransferJobRequest{} }
func (m *UpdateTransferJobRequest) String() string { return proto.CompactTextString(m) }
func (*UpdateTransferJobRequest) ProtoMessage()    {}
func (*UpdateTransferJobRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_transfer_fe1aac113c6727f1, []int{2}
}
func (m *UpdateTransferJobRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UpdateTransferJobRequest.Unmarshal(m, b)
}
func (m *UpdateTransferJobRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UpdateTransferJobRequest.Marshal(b, m, deterministic)
}
func (dst *UpdateTransferJobRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UpdateTransferJobRequest.Merge(dst, src)
}
func (m *UpdateTransferJobRequest) XXX_Size() int {
	return xxx_messageInfo_UpdateTransferJobRequest.Size(m)
}
func (m *UpdateTransferJobRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_UpdateTransferJobRequest.DiscardUnknown(m)
}

var xxx_messageInfo_UpdateTransferJobRequest proto.InternalMessageInfo

func (m *UpdateTransferJobRequest) GetJobName() string {
	if m != nil {
		return m.JobName
	}
	return ""
}

func (m *UpdateTransferJobRequest) GetProjectId() string {
	if m != nil {
		return m.ProjectId
	}
	return ""
}

func (m *UpdateTransferJobRequest) GetTransferJob() *TransferJob {
	if m != nil {
		return m.TransferJob
	}
	return nil
}

func (m *UpdateTransferJobRequest) GetUpdateTransferJobFieldMask() *field_mask.FieldMask {
	if m != nil {
		return m.UpdateTransferJobFieldMask
	}
	return nil
}

// Request passed to GetTransferJob.
type GetTransferJobRequest struct {
	// The job to get.
	// Required.
	JobName string `protobuf:"bytes,1,opt,name=job_name,json=jobName,proto3" json:"job_name,omitempty"`
	// The ID of the Google Cloud Platform Console project that owns the job.
	// Required.
	ProjectId            string   `protobuf:"bytes,2,opt,name=project_id,json=projectId,proto3" json:"project_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetTransferJobRequest) Reset()         { *m = GetTransferJobRequest{} }
func (m *GetTransferJobRequest) String() string { return proto.CompactTextString(m) }
func (*GetTransferJobRequest) ProtoMessage()    {}
func (*GetTransferJobRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_transfer_fe1aac113c6727f1, []int{3}
}
func (m *GetTransferJobRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetTransferJobRequest.Unmarshal(m, b)
}
func (m *GetTransferJobRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetTransferJobRequest.Marshal(b, m, deterministic)
}
func (dst *GetTransferJobRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetTransferJobRequest.Merge(dst, src)
}
func (m *GetTransferJobRequest) XXX_Size() int {
	return xxx_messageInfo_GetTransferJobRequest.Size(m)
}
func (m *GetTransferJobRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetTransferJobRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetTransferJobRequest proto.InternalMessageInfo

func (m *GetTransferJobRequest) GetJobName() string {
	if m != nil {
		return m.JobName
	}
	return ""
}

func (m *GetTransferJobRequest) GetProjectId() string {
	if m != nil {
		return m.ProjectId
	}
	return ""
}

// `project_id`, `job_names`, and `job_statuses` are query parameters that can
// be specified when listing transfer jobs.
type ListTransferJobsRequest struct {
	// A list of query parameters specified as JSON text in the form of
	// {"project_id":"my_project_id",
	// "job_names":["jobid1","jobid2",...],
	// "job_statuses":["status1","status2",...]}.
	// Since `job_names` and `job_statuses` support multiple values, their values
	// must be specified with array notation. `project_id` is required. `job_names`
	// and `job_statuses` are optional.  The valid values for `job_statuses` are
	// case-insensitive: `ENABLED`, `DISABLED`, and `DELETED`.
	Filter string `protobuf:"bytes,1,opt,name=filter,proto3" json:"filter,omitempty"`
	// The list page size. The max allowed value is 256.
	PageSize int32 `protobuf:"varint,4,opt,name=page_size,json=pageSize,proto3" json:"page_size,omitempty"`
	// The list page token.
	PageToken            string   `protobuf:"bytes,5,opt,name=page_token,json=pageToken,proto3" json:"page_token,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListTransferJobsRequest) Reset()         { *m = ListTransferJobsRequest{} }
func (m *ListTransferJobsRequest) String() string { return proto.CompactTextString(m) }
func (*ListTransferJobsRequest) ProtoMessage()    {}
func (*ListTransferJobsRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_transfer_fe1aac113c6727f1, []int{4}
}
func (m *ListTransferJobsRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListTransferJobsRequest.Unmarshal(m, b)
}
func (m *ListTransferJobsRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListTransferJobsRequest.Marshal(b, m, deterministic)
}
func (dst *ListTransferJobsRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListTransferJobsRequest.Merge(dst, src)
}
func (m *ListTransferJobsRequest) XXX_Size() int {
	return xxx_messageInfo_ListTransferJobsRequest.Size(m)
}
func (m *ListTransferJobsRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ListTransferJobsRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ListTransferJobsRequest proto.InternalMessageInfo

func (m *ListTransferJobsRequest) GetFilter() string {
	if m != nil {
		return m.Filter
	}
	return ""
}

func (m *ListTransferJobsRequest) GetPageSize() int32 {
	if m != nil {
		return m.PageSize
	}
	return 0
}

func (m *ListTransferJobsRequest) GetPageToken() string {
	if m != nil {
		return m.PageToken
	}
	return ""
}

// Response from ListTransferJobs.
type ListTransferJobsResponse struct {
	// A list of transfer jobs.
	TransferJobs []*TransferJob `protobuf:"bytes,1,rep,name=transfer_jobs,json=transferJobs,proto3" json:"transfer_jobs,omitempty"`
	// The list next page token.
	NextPageToken        string   `protobuf:"bytes,2,opt,name=next_page_token,json=nextPageToken,proto3" json:"next_page_token,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListTransferJobsResponse) Reset()         { *m = ListTransferJobsResponse{} }
func (m *ListTransferJobsResponse) String() string { return proto.CompactTextString(m) }
func (*ListTransferJobsResponse) ProtoMessage()    {}
func (*ListTransferJobsResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_transfer_fe1aac113c6727f1, []int{5}
}
func (m *ListTransferJobsResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListTransferJobsResponse.Unmarshal(m, b)
}
func (m *ListTransferJobsResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListTransferJobsResponse.Marshal(b, m, deterministic)
}
func (dst *ListTransferJobsResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListTransferJobsResponse.Merge(dst, src)
}
func (m *ListTransferJobsResponse) XXX_Size() int {
	return xxx_messageInfo_ListTransferJobsResponse.Size(m)
}
func (m *ListTransferJobsResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ListTransferJobsResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ListTransferJobsResponse proto.InternalMessageInfo

func (m *ListTransferJobsResponse) GetTransferJobs() []*TransferJob {
	if m != nil {
		return m.TransferJobs
	}
	return nil
}

func (m *ListTransferJobsResponse) GetNextPageToken() string {
	if m != nil {
		return m.NextPageToken
	}
	return ""
}

// Request passed to PauseTransferOperation.
type PauseTransferOperationRequest struct {
	// The name of the transfer operation.
	// Required.
	Name                 string   `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *PauseTransferOperationRequest) Reset()         { *m = PauseTransferOperationRequest{} }
func (m *PauseTransferOperationRequest) String() string { return proto.CompactTextString(m) }
func (*PauseTransferOperationRequest) ProtoMessage()    {}
func (*PauseTransferOperationRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_transfer_fe1aac113c6727f1, []int{6}
}
func (m *PauseTransferOperationRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_PauseTransferOperationRequest.Unmarshal(m, b)
}
func (m *PauseTransferOperationRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_PauseTransferOperationRequest.Marshal(b, m, deterministic)
}
func (dst *PauseTransferOperationRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PauseTransferOperationRequest.Merge(dst, src)
}
func (m *PauseTransferOperationRequest) XXX_Size() int {
	return xxx_messageInfo_PauseTransferOperationRequest.Size(m)
}
func (m *PauseTransferOperationRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_PauseTransferOperationRequest.DiscardUnknown(m)
}

var xxx_messageInfo_PauseTransferOperationRequest proto.InternalMessageInfo

func (m *PauseTransferOperationRequest) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

// Request passed to ResumeTransferOperation.
type ResumeTransferOperationRequest struct {
	// The name of the transfer operation.
	// Required.
	Name                 string   `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ResumeTransferOperationRequest) Reset()         { *m = ResumeTransferOperationRequest{} }
func (m *ResumeTransferOperationRequest) String() string { return proto.CompactTextString(m) }
func (*ResumeTransferOperationRequest) ProtoMessage()    {}
func (*ResumeTransferOperationRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_transfer_fe1aac113c6727f1, []int{7}
}
func (m *ResumeTransferOperationRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ResumeTransferOperationRequest.Unmarshal(m, b)
}
func (m *ResumeTransferOperationRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ResumeTransferOperationRequest.Marshal(b, m, deterministic)
}
func (dst *ResumeTransferOperationRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ResumeTransferOperationRequest.Merge(dst, src)
}
func (m *ResumeTransferOperationRequest) XXX_Size() int {
	return xxx_messageInfo_ResumeTransferOperationRequest.Size(m)
}
func (m *ResumeTransferOperationRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ResumeTransferOperationRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ResumeTransferOperationRequest proto.InternalMessageInfo

func (m *ResumeTransferOperationRequest) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func init() {
	proto.RegisterType((*GetGoogleServiceAccountRequest)(nil), "google.storagetransfer.v1.GetGoogleServiceAccountRequest")
	proto.RegisterType((*CreateTransferJobRequest)(nil), "google.storagetransfer.v1.CreateTransferJobRequest")
	proto.RegisterType((*UpdateTransferJobRequest)(nil), "google.storagetransfer.v1.UpdateTransferJobRequest")
	proto.RegisterType((*GetTransferJobRequest)(nil), "google.storagetransfer.v1.GetTransferJobRequest")
	proto.RegisterType((*ListTransferJobsRequest)(nil), "google.storagetransfer.v1.ListTransferJobsRequest")
	proto.RegisterType((*ListTransferJobsResponse)(nil), "google.storagetransfer.v1.ListTransferJobsResponse")
	proto.RegisterType((*PauseTransferOperationRequest)(nil), "google.storagetransfer.v1.PauseTransferOperationRequest")
	proto.RegisterType((*ResumeTransferOperationRequest)(nil), "google.storagetransfer.v1.ResumeTransferOperationRequest")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// StorageTransferServiceClient is the client API for StorageTransferService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type StorageTransferServiceClient interface {
	// Returns the Google service account that is used by Storage Transfer
	// Service to access buckets in the project where transfers
	// run or in other projects. Each Google service account is associated
	// with one Google Cloud Platform Console project. Users
	// should add this service account to the Google Cloud Storage bucket
	// ACLs to grant access to Storage Transfer Service. This service
	// account is created and owned by Storage Transfer Service and can
	// only be used by Storage Transfer Service.
	GetGoogleServiceAccount(ctx context.Context, in *GetGoogleServiceAccountRequest, opts ...grpc.CallOption) (*GoogleServiceAccount, error)
	// Creates a transfer job that runs periodically.
	CreateTransferJob(ctx context.Context, in *CreateTransferJobRequest, opts ...grpc.CallOption) (*TransferJob, error)
	// Updates a transfer job. Updating a job's transfer spec does not affect
	// transfer operations that are running already. Updating the scheduling
	// of a job is not allowed.
	UpdateTransferJob(ctx context.Context, in *UpdateTransferJobRequest, opts ...grpc.CallOption) (*TransferJob, error)
	// Gets a transfer job.
	GetTransferJob(ctx context.Context, in *GetTransferJobRequest, opts ...grpc.CallOption) (*TransferJob, error)
	// Lists transfer jobs.
	ListTransferJobs(ctx context.Context, in *ListTransferJobsRequest, opts ...grpc.CallOption) (*ListTransferJobsResponse, error)
	// Pauses a transfer operation.
	PauseTransferOperation(ctx context.Context, in *PauseTransferOperationRequest, opts ...grpc.CallOption) (*empty.Empty, error)
	// Resumes a transfer operation that is paused.
	ResumeTransferOperation(ctx context.Context, in *ResumeTransferOperationRequest, opts ...grpc.CallOption) (*empty.Empty, error)
}

type storageTransferServiceClient struct {
	cc *grpc.ClientConn
}

func NewStorageTransferServiceClient(cc *grpc.ClientConn) StorageTransferServiceClient {
	return &storageTransferServiceClient{cc}
}

func (c *storageTransferServiceClient) GetGoogleServiceAccount(ctx context.Context, in *GetGoogleServiceAccountRequest, opts ...grpc.CallOption) (*GoogleServiceAccount, error) {
	out := new(GoogleServiceAccount)
	err := c.cc.Invoke(ctx, "/google.storagetransfer.v1.StorageTransferService/GetGoogleServiceAccount", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storageTransferServiceClient) CreateTransferJob(ctx context.Context, in *CreateTransferJobRequest, opts ...grpc.CallOption) (*TransferJob, error) {
	out := new(TransferJob)
	err := c.cc.Invoke(ctx, "/google.storagetransfer.v1.StorageTransferService/CreateTransferJob", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storageTransferServiceClient) UpdateTransferJob(ctx context.Context, in *UpdateTransferJobRequest, opts ...grpc.CallOption) (*TransferJob, error) {
	out := new(TransferJob)
	err := c.cc.Invoke(ctx, "/google.storagetransfer.v1.StorageTransferService/UpdateTransferJob", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storageTransferServiceClient) GetTransferJob(ctx context.Context, in *GetTransferJobRequest, opts ...grpc.CallOption) (*TransferJob, error) {
	out := new(TransferJob)
	err := c.cc.Invoke(ctx, "/google.storagetransfer.v1.StorageTransferService/GetTransferJob", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storageTransferServiceClient) ListTransferJobs(ctx context.Context, in *ListTransferJobsRequest, opts ...grpc.CallOption) (*ListTransferJobsResponse, error) {
	out := new(ListTransferJobsResponse)
	err := c.cc.Invoke(ctx, "/google.storagetransfer.v1.StorageTransferService/ListTransferJobs", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storageTransferServiceClient) PauseTransferOperation(ctx context.Context, in *PauseTransferOperationRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/google.storagetransfer.v1.StorageTransferService/PauseTransferOperation", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storageTransferServiceClient) ResumeTransferOperation(ctx context.Context, in *ResumeTransferOperationRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/google.storagetransfer.v1.StorageTransferService/ResumeTransferOperation", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// StorageTransferServiceServer is the server API for StorageTransferService service.
type StorageTransferServiceServer interface {
	// Returns the Google service account that is used by Storage Transfer
	// Service to access buckets in the project where transfers
	// run or in other projects. Each Google service account is associated
	// with one Google Cloud Platform Console project. Users
	// should add this service account to the Google Cloud Storage bucket
	// ACLs to grant access to Storage Transfer Service. This service
	// account is created and owned by Storage Transfer Service and can
	// only be used by Storage Transfer Service.
	GetGoogleServiceAccount(context.Context, *GetGoogleServiceAccountRequest) (*GoogleServiceAccount, error)
	// Creates a transfer job that runs periodically.
	CreateTransferJob(context.Context, *CreateTransferJobRequest) (*TransferJob, error)
	// Updates a transfer job. Updating a job's transfer spec does not affect
	// transfer operations that are running already. Updating the scheduling
	// of a job is not allowed.
	UpdateTransferJob(context.Context, *UpdateTransferJobRequest) (*TransferJob, error)
	// Gets a transfer job.
	GetTransferJob(context.Context, *GetTransferJobRequest) (*TransferJob, error)
	// Lists transfer jobs.
	ListTransferJobs(context.Context, *ListTransferJobsRequest) (*ListTransferJobsResponse, error)
	// Pauses a transfer operation.
	PauseTransferOperation(context.Context, *PauseTransferOperationRequest) (*empty.Empty, error)
	// Resumes a transfer operation that is paused.
	ResumeTransferOperation(context.Context, *ResumeTransferOperationRequest) (*empty.Empty, error)
}

func RegisterStorageTransferServiceServer(s *grpc.Server, srv StorageTransferServiceServer) {
	s.RegisterService(&_StorageTransferService_serviceDesc, srv)
}

func _StorageTransferService_GetGoogleServiceAccount_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetGoogleServiceAccountRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageTransferServiceServer).GetGoogleServiceAccount(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.storagetransfer.v1.StorageTransferService/GetGoogleServiceAccount",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageTransferServiceServer).GetGoogleServiceAccount(ctx, req.(*GetGoogleServiceAccountRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorageTransferService_CreateTransferJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateTransferJobRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageTransferServiceServer).CreateTransferJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.storagetransfer.v1.StorageTransferService/CreateTransferJob",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageTransferServiceServer).CreateTransferJob(ctx, req.(*CreateTransferJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorageTransferService_UpdateTransferJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateTransferJobRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageTransferServiceServer).UpdateTransferJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.storagetransfer.v1.StorageTransferService/UpdateTransferJob",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageTransferServiceServer).UpdateTransferJob(ctx, req.(*UpdateTransferJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorageTransferService_GetTransferJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetTransferJobRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageTransferServiceServer).GetTransferJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.storagetransfer.v1.StorageTransferService/GetTransferJob",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageTransferServiceServer).GetTransferJob(ctx, req.(*GetTransferJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorageTransferService_ListTransferJobs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListTransferJobsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageTransferServiceServer).ListTransferJobs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.storagetransfer.v1.StorageTransferService/ListTransferJobs",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageTransferServiceServer).ListTransferJobs(ctx, req.(*ListTransferJobsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorageTransferService_PauseTransferOperation_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PauseTransferOperationRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageTransferServiceServer).PauseTransferOperation(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.storagetransfer.v1.StorageTransferService/PauseTransferOperation",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageTransferServiceServer).PauseTransferOperation(ctx, req.(*PauseTransferOperationRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorageTransferService_ResumeTransferOperation_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ResumeTransferOperationRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageTransferServiceServer).ResumeTransferOperation(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/google.storagetransfer.v1.StorageTransferService/ResumeTransferOperation",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageTransferServiceServer).ResumeTransferOperation(ctx, req.(*ResumeTransferOperationRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _StorageTransferService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "google.storagetransfer.v1.StorageTransferService",
	HandlerType: (*StorageTransferServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetGoogleServiceAccount",
			Handler:    _StorageTransferService_GetGoogleServiceAccount_Handler,
		},
		{
			MethodName: "CreateTransferJob",
			Handler:    _StorageTransferService_CreateTransferJob_Handler,
		},
		{
			MethodName: "UpdateTransferJob",
			Handler:    _StorageTransferService_UpdateTransferJob_Handler,
		},
		{
			MethodName: "GetTransferJob",
			Handler:    _StorageTransferService_GetTransferJob_Handler,
		},
		{
			MethodName: "ListTransferJobs",
			Handler:    _StorageTransferService_ListTransferJobs_Handler,
		},
		{
			MethodName: "PauseTransferOperation",
			Handler:    _StorageTransferService_PauseTransferOperation_Handler,
		},
		{
			MethodName: "ResumeTransferOperation",
			Handler:    _StorageTransferService_ResumeTransferOperation_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "google/storagetransfer/v1/transfer.proto",
}

func init() {
	proto.RegisterFile("google/storagetransfer/v1/transfer.proto", fileDescriptor_transfer_fe1aac113c6727f1)
}

var fileDescriptor_transfer_fe1aac113c6727f1 = []byte{
	// 786 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xac, 0x56, 0xdf, 0x4e, 0x13, 0x4f,
	0x14, 0xce, 0xf0, 0xef, 0x07, 0x03, 0xfc, 0x84, 0x49, 0x2c, 0x4b, 0x91, 0xda, 0x2c, 0x49, 0xc5,
	0x6a, 0x76, 0x6d, 0xeb, 0x85, 0x62, 0x8c, 0x11, 0xa2, 0x88, 0x7f, 0xb1, 0x20, 0x17, 0x86, 0xb8,
	0x99, 0xb6, 0xd3, 0xcd, 0x96, 0x76, 0x67, 0xdd, 0x99, 0x25, 0x02, 0xe1, 0xc6, 0x17, 0x30, 0xd1,
	0x98, 0x98, 0x18, 0x13, 0xaf, 0xbd, 0xf7, 0x11, 0xbc, 0xf1, 0xd6, 0x57, 0xf0, 0x21, 0xf4, 0x4a,
	0xb3, 0xb3, 0xb3, 0x65, 0xd9, 0xb6, 0x0b, 0xa8, 0x77, 0x3b, 0x33, 0xe7, 0x9c, 0xef, 0x3b, 0xe7,
	0xf0, 0x7d, 0x14, 0xce, 0x9b, 0x94, 0x9a, 0x4d, 0xa2, 0x33, 0x4e, 0x5d, 0x6c, 0x12, 0xee, 0x62,
	0x9b, 0xd5, 0x89, 0xab, 0x6f, 0x17, 0xf4, 0xf0, 0x5b, 0x73, 0x5c, 0xca, 0x29, 0x9a, 0x0e, 0x22,
	0xb5, 0x58, 0xa4, 0xb6, 0x5d, 0x48, 0x9f, 0x91, 0x45, 0xb0, 0x63, 0xe9, 0xd8, 0xb6, 0x29, 0xc7,
	0xdc, 0xa2, 0x36, 0x0b, 0x12, 0xd3, 0x33, 0xf2, 0x55, 0x9c, 0x2a, 0x5e, 0x5d, 0x27, 0x2d, 0x87,
	0xef, 0xc8, 0xc7, 0x6c, 0xfc, 0xb1, 0x6e, 0x91, 0x66, 0xcd, 0x68, 0x61, 0xb6, 0x25, 0x23, 0xb4,
	0xa3, 0x19, 0x1a, 0x7c, 0xc7, 0x21, 0x12, 0x4e, 0xbd, 0x01, 0x33, 0xcb, 0x84, 0x2f, 0x8b, 0xa4,
	0x35, 0xe2, 0x6e, 0x5b, 0x55, 0x72, 0xb3, 0x5a, 0xa5, 0x9e, 0xcd, 0xcb, 0xe4, 0xb9, 0x47, 0x18,
	0x47, 0xb3, 0x10, 0x3a, 0x2e, 0x6d, 0x90, 0x2a, 0x37, 0xac, 0x9a, 0x02, 0xb2, 0x60, 0x7e, 0xa4,
	0x3c, 0x22, 0x6f, 0x56, 0x6a, 0x2a, 0x81, 0xca, 0x92, 0x4b, 0x30, 0x27, 0xeb, 0xb2, 0xfc, 0x5d,
	0x5a, 0x09, 0x53, 0x57, 0xe0, 0x58, 0x1b, 0xb4, 0x41, 0x2b, 0x22, 0x79, 0xb4, 0x98, 0xd3, 0x7a,
	0xce, 0x46, 0x8b, 0x16, 0x19, 0xe5, 0x07, 0x07, 0xf5, 0x17, 0x80, 0xca, 0x13, 0xa7, 0xd6, 0x1d,
	0x67, 0x1a, 0x0e, 0x37, 0x68, 0xc5, 0xb0, 0x71, 0x8b, 0x48, 0x82, 0xff, 0x35, 0x68, 0xe5, 0x21,
	0x6e, 0x91, 0x18, 0xfb, 0xbe, 0x18, 0xfb, 0x0e, 0x86, 0xfd, 0x7f, 0xcc, 0x10, 0x3d, 0x83, 0x19,
	0x4f, 0x10, 0x34, 0xa2, 0x15, 0x8d, 0x83, 0x0d, 0x29, 0x03, 0xa2, 0x78, 0xb8, 0x22, 0x2d, 0x5c,
	0xa2, 0x76, 0xdb, 0x0f, 0x79, 0x80, 0xd9, 0x56, 0x39, 0xed, 0xc5, 0x5b, 0x6c, 0xbf, 0xa9, 0x8f,
	0xe1, 0xe9, 0x65, 0xc2, 0xff, 0x65, 0xf7, 0x6a, 0x0b, 0x4e, 0xdd, 0xb7, 0x58, 0xb4, 0x26, 0x0b,
	0x8b, 0xa6, 0xe0, 0x50, 0xdd, 0x6a, 0x72, 0xe2, 0xca, 0x92, 0xf2, 0x84, 0x66, 0xe0, 0x88, 0x83,
	0x4d, 0x62, 0x30, 0x6b, 0x97, 0x88, 0x86, 0x06, 0xcb, 0xc3, 0xfe, 0xc5, 0x9a, 0xb5, 0x1b, 0xc0,
	0xf9, 0x8f, 0x9c, 0x6e, 0x11, 0x5b, 0x19, 0x94, 0x70, 0xd8, 0x24, 0xeb, 0xfe, 0x85, 0xfa, 0x0a,
	0x40, 0xa5, 0x13, 0x8f, 0x39, 0xd4, 0x66, 0x04, 0xdd, 0x83, 0xe3, 0xd1, 0xb9, 0x31, 0x05, 0x64,
	0xfb, 0x4f, 0xb0, 0x8a, 0xb1, 0xc8, 0x2a, 0x18, 0xca, 0xc1, 0x53, 0x36, 0x79, 0xc1, 0x8d, 0x08,
	0x9b, 0xa0, 0xf9, 0x71, 0xff, 0x7a, 0xb5, 0xcd, 0xa8, 0x04, 0x67, 0x57, 0xb1, 0xc7, 0xda, 0x03,
	0x7f, 0xe4, 0x10, 0x57, 0xa8, 0x31, 0x1c, 0x03, 0x82, 0x03, 0x91, 0xb9, 0x8a, 0x6f, 0xf5, 0x32,
	0xcc, 0x94, 0x09, 0xf3, 0x5a, 0x27, 0xca, 0x2a, 0xfe, 0x1c, 0x86, 0xa9, 0xb5, 0xa0, 0x87, 0x30,
	0x4f, 0xea, 0x0d, 0x7d, 0x06, 0x70, 0xaa, 0x87, 0x08, 0xd1, 0xd5, 0x84, 0xfe, 0x93, 0x85, 0x9b,
	0xd6, 0x93, 0x52, 0xbb, 0xe4, 0xa9, 0xda, 0xcb, 0x6f, 0xdf, 0xdf, 0xf4, 0xcd, 0xa3, 0x9c, 0xef,
	0x16, 0x66, 0x97, 0x08, 0xa6, 0xef, 0x1d, 0xfc, 0x39, 0xed, 0xa3, 0x77, 0x00, 0x4e, 0x76, 0x68,
	0x1f, 0x95, 0x12, 0x60, 0x7b, 0x39, 0x45, 0xfa, 0x98, 0x6b, 0x56, 0x73, 0x82, 0x62, 0x56, 0x9d,
	0x88, 0x1a, 0x9a, 0xbf, 0xf2, 0x85, 0x43, 0x3a, 0x46, 0xef, 0x01, 0x9c, 0xec, 0xb0, 0x8b, 0x44,
	0x6a, 0xbd, 0xcc, 0xe5, 0xd8, 0xd4, 0xce, 0x0b, 0x6a, 0x73, 0xc5, 0x8c, 0x4f, 0x6d, 0x2f, 0x54,
	0xe4, 0xf5, 0x28, 0x49, 0x3d, 0x9f, 0xdf, 0x5f, 0x00, 0x79, 0xf4, 0x1a, 0xc0, 0xff, 0x0f, 0x6b,
	0x19, 0x5d, 0x4a, 0xde, 0xf3, 0xdf, 0x8f, 0x0c, 0x1d, 0xc1, 0x0b, 0xbd, 0x05, 0x70, 0x22, 0xae,
	0x4e, 0x54, 0x4c, 0x00, 0xe9, 0x61, 0x1d, 0xe9, 0xd2, 0x89, 0x72, 0x02, 0xf9, 0xab, 0x8a, 0x60,
	0x89, 0x50, 0xc7, 0x62, 0xd1, 0x07, 0x00, 0x53, 0xdd, 0x45, 0x8a, 0xae, 0x24, 0x20, 0x25, 0xea,
	0x3a, 0x9d, 0xea, 0x30, 0xe1, 0x5b, 0xfe, 0xbf, 0x59, 0xb5, 0x20, 0x68, 0x5c, 0x50, 0x85, 0x04,
	0xf6, 0x0e, 0x0d, 0xaa, 0x5d, 0x23, 0x58, 0xa3, 0xe3, 0xd7, 0xf7, 0x97, 0xf9, 0x11, 0xc0, 0xa9,
	0x1e, 0x7e, 0x90, 0xa8, 0xde, 0x64, 0x0f, 0xe9, 0xc9, 0xb0, 0x28, 0x18, 0x5e, 0x54, 0xcf, 0x1d,
	0xc9, 0xd0, 0x15, 0x00, 0x0b, 0x20, 0xbf, 0xf8, 0x05, 0xc0, 0xb9, 0x2a, 0x6d, 0x25, 0x90, 0x11,
	0x20, 0x8b, 0xe3, 0x21, 0x9b, 0x55, 0xff, 0xf8, 0xf4, 0x8e, 0x8c, 0x37, 0x69, 0x13, 0xdb, 0xa6,
	0x46, 0x5d, 0x53, 0x37, 0x89, 0x2d, 0x42, 0xa5, 0x3d, 0x60, 0xc7, 0x62, 0x5d, 0x7e, 0x6a, 0x5c,
	0x8b, 0x5d, 0xfd, 0x00, 0xe0, 0x53, 0xdf, 0xd9, 0xc0, 0x73, 0xb4, 0xa5, 0x26, 0xf5, 0x6a, 0x5a,
	0xcc, 0x0a, 0xb5, 0x8d, 0xc2, 0xd7, 0x30, 0x62, 0x53, 0x44, 0x6c, 0xc6, 0x22, 0x36, 0x37, 0x0a,
	0x95, 0x21, 0x81, 0x5d, 0xfa, 0x1d, 0x00, 0x00, 0xff, 0xff, 0x9e, 0x71, 0x04, 0x73, 0x87, 0x09,
	0x00, 0x00,
}
