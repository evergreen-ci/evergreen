// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package glacier provides the client and types for making API
// requests to Amazon Glacier.
//
// Amazon Glacier is a storage solution for "cold data."
//
// Amazon Glacier is an extremely low-cost storage service that provides secure,
// durable, and easy-to-use storage for data backup and archival. With Amazon
// Glacier, customers can store their data cost effectively for months, years,
// or decades. Amazon Glacier also enables customers to offload the administrative
// burdens of operating and scaling storage to AWS, so they don't have to worry
// about capacity planning, hardware provisioning, data replication, hardware
// failure and recovery, or time-consuming hardware migrations.
//
// Amazon Glacier is a great storage choice when low storage cost is paramount,
// your data is rarely retrieved, and retrieval latency of several hours is
// acceptable. If your application requires fast or frequent access to your
// data, consider using Amazon S3. For more information, see Amazon Simple Storage
// Service (Amazon S3) (http://aws.amazon.com/s3/).
//
// You can store any kind of data in any format. There is no maximum limit on
// the total amount of data you can store in Amazon Glacier.
//
// If you are a first-time user of Amazon Glacier, we recommend that you begin
// by reading the following sections in the Amazon Glacier Developer Guide:
//
//    * What is Amazon Glacier (http://docs.aws.amazon.com/amazonglacier/latest/dev/introduction.html)
//    - This section of the Developer Guide describes the underlying data model,
//    the operations it supports, and the AWS SDKs that you can use to interact
//    with the service.
//
//    * Getting Started with Amazon Glacier (http://docs.aws.amazon.com/amazonglacier/latest/dev/amazon-glacier-getting-started.html)
//    - The Getting Started section walks you through the process of creating
//    a vault, uploading archives, creating jobs to download archives, retrieving
//    the job output, and deleting archives.
//
// See glacier package documentation for more information.
// https://docs.aws.amazon.com/sdk-for-go/api/service/glacier/
//
// Using the Client
//
// To Amazon Glacier with the SDK use the New function to create
// a new service client. With that client you can make API requests to the service.
// These clients are safe to use concurrently.
//
// See the SDK's documentation for more information on how to use the SDK.
// https://docs.aws.amazon.com/sdk-for-go/api/
//
// See aws.Config documentation for more information on configuring SDK clients.
// https://docs.aws.amazon.com/sdk-for-go/api/aws/#Config
//
// See the Amazon Glacier client Glacier for more
// information on creating client for this service.
// https://docs.aws.amazon.com/sdk-for-go/api/service/glacier/#New
package glacier
