// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package rds provides the client and types for making API
// requests to Amazon Relational Database Service.
//
// Amazon Relational Database Service (Amazon RDS) is a web service that makes
// it easier to set up, operate, and scale a relational database in the cloud.
// It provides cost-efficient, resizable capacity for an industry-standard relational
// database and manages common database administration tasks, freeing up developers
// to focus on what makes their applications and businesses unique.
//
// Amazon RDS gives you access to the capabilities of a MySQL, MariaDB, PostgreSQL,
// Microsoft SQL Server, Oracle, or Amazon Aurora database server. These capabilities
// mean that the code, applications, and tools you already use today with your
// existing databases work with Amazon RDS without modification. Amazon RDS
// automatically backs up your database and maintains the database software
// that powers your DB instance. Amazon RDS is flexible: you can scale your
// database instance's compute resources and storage capacity to meet your application's
// demand. As with all Amazon Web Services, there are no up-front investments,
// and you pay only for the resources you use.
//
// This interface reference for Amazon RDS contains documentation for a programming
// or command line interface you can use to manage Amazon RDS. Note that Amazon
// RDS is asynchronous, which means that some interfaces might require techniques
// such as polling or callback functions to determine when a command has been
// applied. In this reference, the parameter descriptions indicate whether a
// command is applied immediately, on the next instance reboot, or during the
// maintenance window. The reference structure is as follows, and we list following
// some related topics from the user guide.
//
// Amazon RDS API Reference
//
//    * For the alphabetical list of API actions, see API Actions (http://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_Operations.html).
//
//    * For the alphabetical list of data types, see Data Types (http://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_Types.html).
//
//    * For a list of common query parameters, see Common Parameters (http://docs.aws.amazon.com/AmazonRDS/latest/APIReference/CommonParameters.html).
//
//    * For descriptions of the error codes, see Common Errors (http://docs.aws.amazon.com/AmazonRDS/latest/APIReference/CommonErrors.html).
//
// Amazon RDS User Guide
//
//    * For a summary of the Amazon RDS interfaces, see Available RDS Interfaces
//    (http://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Welcome.html#Welcome.Interfaces).
//
//    * For more information about how to use the Query API, see Using the Query
//    API (http://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Using_the_Query_API.html).
//
// See https://docs.aws.amazon.com/goto/WebAPI/rds-2014-10-31 for more information on this service.
//
// See rds package documentation for more information.
// https://docs.aws.amazon.com/sdk-for-go/api/service/rds/
//
// Using the Client
//
// To Amazon Relational Database Service with the SDK use the New function to create
// a new service client. With that client you can make API requests to the service.
// These clients are safe to use concurrently.
//
// See the SDK's documentation for more information on how to use the SDK.
// https://docs.aws.amazon.com/sdk-for-go/api/
//
// See aws.Config documentation for more information on configuring SDK clients.
// https://docs.aws.amazon.com/sdk-for-go/api/aws/#Config
//
// See the Amazon Relational Database Service client RDS for more
// information on creating client for this service.
// https://docs.aws.amazon.com/sdk-for-go/api/service/rds/#New
package rds
