// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package ssm

const (

	// ErrCodeAlreadyExistsException for service response error code
	// "AlreadyExistsException".
	//
	// Error returned if an attempt is made to register a patch group with a patch
	// baseline that is already registered with a different patch baseline.
	ErrCodeAlreadyExistsException = "AlreadyExistsException"

	// ErrCodeAssociatedInstances for service response error code
	// "AssociatedInstances".
	//
	// You must disassociate a document from all instances before you can delete
	// it.
	ErrCodeAssociatedInstances = "AssociatedInstances"

	// ErrCodeAssociationAlreadyExists for service response error code
	// "AssociationAlreadyExists".
	//
	// The specified association already exists.
	ErrCodeAssociationAlreadyExists = "AssociationAlreadyExists"

	// ErrCodeAssociationDoesNotExist for service response error code
	// "AssociationDoesNotExist".
	//
	// The specified association does not exist.
	ErrCodeAssociationDoesNotExist = "AssociationDoesNotExist"

	// ErrCodeAssociationExecutionDoesNotExist for service response error code
	// "AssociationExecutionDoesNotExist".
	//
	// The specified execution ID does not exist. Verify the ID number and try again.
	ErrCodeAssociationExecutionDoesNotExist = "AssociationExecutionDoesNotExist"

	// ErrCodeAssociationLimitExceeded for service response error code
	// "AssociationLimitExceeded".
	//
	// You can have at most 2,000 active associations.
	ErrCodeAssociationLimitExceeded = "AssociationLimitExceeded"

	// ErrCodeAssociationVersionLimitExceeded for service response error code
	// "AssociationVersionLimitExceeded".
	//
	// You have reached the maximum number versions allowed for an association.
	// Each association has a limit of 1,000 versions.
	ErrCodeAssociationVersionLimitExceeded = "AssociationVersionLimitExceeded"

	// ErrCodeAutomationDefinitionNotFoundException for service response error code
	// "AutomationDefinitionNotFoundException".
	//
	// An Automation document with the specified name could not be found.
	ErrCodeAutomationDefinitionNotFoundException = "AutomationDefinitionNotFoundException"

	// ErrCodeAutomationDefinitionVersionNotFoundException for service response error code
	// "AutomationDefinitionVersionNotFoundException".
	//
	// An Automation document with the specified name and version could not be found.
	ErrCodeAutomationDefinitionVersionNotFoundException = "AutomationDefinitionVersionNotFoundException"

	// ErrCodeAutomationExecutionLimitExceededException for service response error code
	// "AutomationExecutionLimitExceededException".
	//
	// The number of simultaneously running Automation executions exceeded the allowable
	// limit.
	ErrCodeAutomationExecutionLimitExceededException = "AutomationExecutionLimitExceededException"

	// ErrCodeAutomationExecutionNotFoundException for service response error code
	// "AutomationExecutionNotFoundException".
	//
	// There is no automation execution information for the requested automation
	// execution ID.
	ErrCodeAutomationExecutionNotFoundException = "AutomationExecutionNotFoundException"

	// ErrCodeAutomationStepNotFoundException for service response error code
	// "AutomationStepNotFoundException".
	//
	// The specified step name and execution ID don't exist. Verify the information
	// and try again.
	ErrCodeAutomationStepNotFoundException = "AutomationStepNotFoundException"

	// ErrCodeComplianceTypeCountLimitExceededException for service response error code
	// "ComplianceTypeCountLimitExceededException".
	//
	// You specified too many custom compliance types. You can specify a maximum
	// of 10 different types.
	ErrCodeComplianceTypeCountLimitExceededException = "ComplianceTypeCountLimitExceededException"

	// ErrCodeCustomSchemaCountLimitExceededException for service response error code
	// "CustomSchemaCountLimitExceededException".
	//
	// You have exceeded the limit for custom schemas. Delete one or more custom
	// schemas and try again.
	ErrCodeCustomSchemaCountLimitExceededException = "CustomSchemaCountLimitExceededException"

	// ErrCodeDocumentAlreadyExists for service response error code
	// "DocumentAlreadyExists".
	//
	// The specified document already exists.
	ErrCodeDocumentAlreadyExists = "DocumentAlreadyExists"

	// ErrCodeDocumentLimitExceeded for service response error code
	// "DocumentLimitExceeded".
	//
	// You can have at most 200 active Systems Manager documents.
	ErrCodeDocumentLimitExceeded = "DocumentLimitExceeded"

	// ErrCodeDocumentPermissionLimit for service response error code
	// "DocumentPermissionLimit".
	//
	// The document cannot be shared with more AWS user accounts. You can share
	// a document with a maximum of 20 accounts. You can publicly share up to five
	// documents. If you need to increase this limit, contact AWS Support.
	ErrCodeDocumentPermissionLimit = "DocumentPermissionLimit"

	// ErrCodeDocumentVersionLimitExceeded for service response error code
	// "DocumentVersionLimitExceeded".
	//
	// The document has too many versions. Delete one or more document versions
	// and try again.
	ErrCodeDocumentVersionLimitExceeded = "DocumentVersionLimitExceeded"

	// ErrCodeDoesNotExistException for service response error code
	// "DoesNotExistException".
	//
	// Error returned when the ID specified for a resource, such as a maintenance
	// window or Patch baseline, doesn't exist.
	//
	// For information about resource limits in Systems Manager, see AWS Systems
	// Manager Limits (http://docs.aws.amazon.com/general/latest/gr/aws_service_limits.html#limits_ssm).
	ErrCodeDoesNotExistException = "DoesNotExistException"

	// ErrCodeDuplicateDocumentContent for service response error code
	// "DuplicateDocumentContent".
	//
	// The content of the association document matches another document. Change
	// the content of the document and try again.
	ErrCodeDuplicateDocumentContent = "DuplicateDocumentContent"

	// ErrCodeDuplicateDocumentVersionName for service response error code
	// "DuplicateDocumentVersionName".
	//
	// The version name has already been used in this document. Specify a different
	// version name, and then try again.
	ErrCodeDuplicateDocumentVersionName = "DuplicateDocumentVersionName"

	// ErrCodeDuplicateInstanceId for service response error code
	// "DuplicateInstanceId".
	//
	// You cannot specify an instance ID in more than one association.
	ErrCodeDuplicateInstanceId = "DuplicateInstanceId"

	// ErrCodeFeatureNotAvailableException for service response error code
	// "FeatureNotAvailableException".
	//
	// You attempted to register a LAMBDA or STEP_FUNCTION task in a region where
	// the corresponding service is not available.
	ErrCodeFeatureNotAvailableException = "FeatureNotAvailableException"

	// ErrCodeHierarchyLevelLimitExceededException for service response error code
	// "HierarchyLevelLimitExceededException".
	//
	// A hierarchy can have a maximum of 15 levels. For more information, see Requirements
	// and Constraints for Parameter Names (http://docs.aws.amazon.com/systems-manager/latest/userguide/sysman-parameter-name-constraints.html)
	// in the AWS Systems Manager User Guide.
	ErrCodeHierarchyLevelLimitExceededException = "HierarchyLevelLimitExceededException"

	// ErrCodeHierarchyTypeMismatchException for service response error code
	// "HierarchyTypeMismatchException".
	//
	// Parameter Store does not support changing a parameter type in a hierarchy.
	// For example, you can't change a parameter from a String type to a SecureString
	// type. You must create a new, unique parameter.
	ErrCodeHierarchyTypeMismatchException = "HierarchyTypeMismatchException"

	// ErrCodeIdempotentParameterMismatch for service response error code
	// "IdempotentParameterMismatch".
	//
	// Error returned when an idempotent operation is retried and the parameters
	// don't match the original call to the API with the same idempotency token.
	ErrCodeIdempotentParameterMismatch = "IdempotentParameterMismatch"

	// ErrCodeIncompatiblePolicyException for service response error code
	// "IncompatiblePolicyException".
	//
	// There is a conflict in the policies specified for this parameter. You can't,
	// for example, specify two Expiration policies for a parameter. Review your
	// policies, and try again.
	ErrCodeIncompatiblePolicyException = "IncompatiblePolicyException"

	// ErrCodeInternalServerError for service response error code
	// "InternalServerError".
	//
	// An error occurred on the server side.
	ErrCodeInternalServerError = "InternalServerError"

	// ErrCodeInvalidActivation for service response error code
	// "InvalidActivation".
	//
	// The activation is not valid. The activation might have been deleted, or the
	// ActivationId and the ActivationCode do not match.
	ErrCodeInvalidActivation = "InvalidActivation"

	// ErrCodeInvalidActivationId for service response error code
	// "InvalidActivationId".
	//
	// The activation ID is not valid. Verify the you entered the correct ActivationId
	// or ActivationCode and try again.
	ErrCodeInvalidActivationId = "InvalidActivationId"

	// ErrCodeInvalidAggregatorException for service response error code
	// "InvalidAggregatorException".
	//
	// The specified aggregator is not valid for inventory groups. Verify that the
	// aggregator uses a valid inventory type such as AWS:Application or AWS:InstanceInformation.
	ErrCodeInvalidAggregatorException = "InvalidAggregatorException"

	// ErrCodeInvalidAllowedPatternException for service response error code
	// "InvalidAllowedPatternException".
	//
	// The request does not meet the regular expression requirement.
	ErrCodeInvalidAllowedPatternException = "InvalidAllowedPatternException"

	// ErrCodeInvalidAssociation for service response error code
	// "InvalidAssociation".
	//
	// The association is not valid or does not exist.
	ErrCodeInvalidAssociation = "InvalidAssociation"

	// ErrCodeInvalidAssociationVersion for service response error code
	// "InvalidAssociationVersion".
	//
	// The version you specified is not valid. Use ListAssociationVersions to view
	// all versions of an association according to the association ID. Or, use the
	// $LATEST parameter to view the latest version of the association.
	ErrCodeInvalidAssociationVersion = "InvalidAssociationVersion"

	// ErrCodeInvalidAutomationExecutionParametersException for service response error code
	// "InvalidAutomationExecutionParametersException".
	//
	// The supplied parameters for invoking the specified Automation document are
	// incorrect. For example, they may not match the set of parameters permitted
	// for the specified Automation document.
	ErrCodeInvalidAutomationExecutionParametersException = "InvalidAutomationExecutionParametersException"

	// ErrCodeInvalidAutomationSignalException for service response error code
	// "InvalidAutomationSignalException".
	//
	// The signal is not valid for the current Automation execution.
	ErrCodeInvalidAutomationSignalException = "InvalidAutomationSignalException"

	// ErrCodeInvalidAutomationStatusUpdateException for service response error code
	// "InvalidAutomationStatusUpdateException".
	//
	// The specified update status operation is not valid.
	ErrCodeInvalidAutomationStatusUpdateException = "InvalidAutomationStatusUpdateException"

	// ErrCodeInvalidCommandId for service response error code
	// "InvalidCommandId".
	ErrCodeInvalidCommandId = "InvalidCommandId"

	// ErrCodeInvalidDeleteInventoryParametersException for service response error code
	// "InvalidDeleteInventoryParametersException".
	//
	// One or more of the parameters specified for the delete operation is not valid.
	// Verify all parameters and try again.
	ErrCodeInvalidDeleteInventoryParametersException = "InvalidDeleteInventoryParametersException"

	// ErrCodeInvalidDeletionIdException for service response error code
	// "InvalidDeletionIdException".
	//
	// The ID specified for the delete operation does not exist or is not valid.
	// Verify the ID and try again.
	ErrCodeInvalidDeletionIdException = "InvalidDeletionIdException"

	// ErrCodeInvalidDocument for service response error code
	// "InvalidDocument".
	//
	// The specified document does not exist.
	ErrCodeInvalidDocument = "InvalidDocument"

	// ErrCodeInvalidDocumentContent for service response error code
	// "InvalidDocumentContent".
	//
	// The content for the document is not valid.
	ErrCodeInvalidDocumentContent = "InvalidDocumentContent"

	// ErrCodeInvalidDocumentOperation for service response error code
	// "InvalidDocumentOperation".
	//
	// You attempted to delete a document while it is still shared. You must stop
	// sharing the document before you can delete it.
	ErrCodeInvalidDocumentOperation = "InvalidDocumentOperation"

	// ErrCodeInvalidDocumentSchemaVersion for service response error code
	// "InvalidDocumentSchemaVersion".
	//
	// The version of the document schema is not supported.
	ErrCodeInvalidDocumentSchemaVersion = "InvalidDocumentSchemaVersion"

	// ErrCodeInvalidDocumentVersion for service response error code
	// "InvalidDocumentVersion".
	//
	// The document version is not valid or does not exist.
	ErrCodeInvalidDocumentVersion = "InvalidDocumentVersion"

	// ErrCodeInvalidFilter for service response error code
	// "InvalidFilter".
	//
	// The filter name is not valid. Verify the you entered the correct name and
	// try again.
	ErrCodeInvalidFilter = "InvalidFilter"

	// ErrCodeInvalidFilterKey for service response error code
	// "InvalidFilterKey".
	//
	// The specified key is not valid.
	ErrCodeInvalidFilterKey = "InvalidFilterKey"

	// ErrCodeInvalidFilterOption for service response error code
	// "InvalidFilterOption".
	//
	// The specified filter option is not valid. Valid options are Equals and BeginsWith.
	// For Path filter, valid options are Recursive and OneLevel.
	ErrCodeInvalidFilterOption = "InvalidFilterOption"

	// ErrCodeInvalidFilterValue for service response error code
	// "InvalidFilterValue".
	//
	// The filter value is not valid. Verify the value and try again.
	ErrCodeInvalidFilterValue = "InvalidFilterValue"

	// ErrCodeInvalidInstanceId for service response error code
	// "InvalidInstanceId".
	//
	// The following problems can cause this exception:
	//
	// You do not have permission to access the instance.
	//
	// SSM Agent is not running. On managed instances and Linux instances, verify
	// that the SSM Agent is running. On EC2 Windows instances, verify that the
	// EC2Config service is running.
	//
	// SSM Agent or EC2Config service is not registered to the SSM endpoint. Try
	// reinstalling SSM Agent or EC2Config service.
	//
	// The instance is not in valid state. Valid states are: Running, Pending, Stopped,
	// Stopping. Invalid states are: Shutting-down and Terminated.
	ErrCodeInvalidInstanceId = "InvalidInstanceId"

	// ErrCodeInvalidInstanceInformationFilterValue for service response error code
	// "InvalidInstanceInformationFilterValue".
	//
	// The specified filter value is not valid.
	ErrCodeInvalidInstanceInformationFilterValue = "InvalidInstanceInformationFilterValue"

	// ErrCodeInvalidInventoryGroupException for service response error code
	// "InvalidInventoryGroupException".
	//
	// The specified inventory group is not valid.
	ErrCodeInvalidInventoryGroupException = "InvalidInventoryGroupException"

	// ErrCodeInvalidInventoryItemContextException for service response error code
	// "InvalidInventoryItemContextException".
	//
	// You specified invalid keys or values in the Context attribute for InventoryItem.
	// Verify the keys and values, and try again.
	ErrCodeInvalidInventoryItemContextException = "InvalidInventoryItemContextException"

	// ErrCodeInvalidInventoryRequestException for service response error code
	// "InvalidInventoryRequestException".
	//
	// The request is not valid.
	ErrCodeInvalidInventoryRequestException = "InvalidInventoryRequestException"

	// ErrCodeInvalidItemContentException for service response error code
	// "InvalidItemContentException".
	//
	// One or more content items is not valid.
	ErrCodeInvalidItemContentException = "InvalidItemContentException"

	// ErrCodeInvalidKeyId for service response error code
	// "InvalidKeyId".
	//
	// The query key ID is not valid.
	ErrCodeInvalidKeyId = "InvalidKeyId"

	// ErrCodeInvalidNextToken for service response error code
	// "InvalidNextToken".
	//
	// The specified token is not valid.
	ErrCodeInvalidNextToken = "InvalidNextToken"

	// ErrCodeInvalidNotificationConfig for service response error code
	// "InvalidNotificationConfig".
	//
	// One or more configuration items is not valid. Verify that a valid Amazon
	// Resource Name (ARN) was provided for an Amazon SNS topic.
	ErrCodeInvalidNotificationConfig = "InvalidNotificationConfig"

	// ErrCodeInvalidOptionException for service response error code
	// "InvalidOptionException".
	//
	// The delete inventory option specified is not valid. Verify the option and
	// try again.
	ErrCodeInvalidOptionException = "InvalidOptionException"

	// ErrCodeInvalidOutputFolder for service response error code
	// "InvalidOutputFolder".
	//
	// The S3 bucket does not exist.
	ErrCodeInvalidOutputFolder = "InvalidOutputFolder"

	// ErrCodeInvalidOutputLocation for service response error code
	// "InvalidOutputLocation".
	//
	// The output location is not valid or does not exist.
	ErrCodeInvalidOutputLocation = "InvalidOutputLocation"

	// ErrCodeInvalidParameters for service response error code
	// "InvalidParameters".
	//
	// You must specify values for all required parameters in the Systems Manager
	// document. You can only supply values to parameters defined in the Systems
	// Manager document.
	ErrCodeInvalidParameters = "InvalidParameters"

	// ErrCodeInvalidPermissionType for service response error code
	// "InvalidPermissionType".
	//
	// The permission type is not supported. Share is the only supported permission
	// type.
	ErrCodeInvalidPermissionType = "InvalidPermissionType"

	// ErrCodeInvalidPluginName for service response error code
	// "InvalidPluginName".
	//
	// The plugin name is not valid.
	ErrCodeInvalidPluginName = "InvalidPluginName"

	// ErrCodeInvalidPolicyAttributeException for service response error code
	// "InvalidPolicyAttributeException".
	//
	// A policy attribute or its value is invalid.
	ErrCodeInvalidPolicyAttributeException = "InvalidPolicyAttributeException"

	// ErrCodeInvalidPolicyTypeException for service response error code
	// "InvalidPolicyTypeException".
	//
	// The policy type is not supported. Parameter Store supports the following
	// policy types: Expiration, ExpirationNotification, and NoChangeNotification.
	ErrCodeInvalidPolicyTypeException = "InvalidPolicyTypeException"

	// ErrCodeInvalidResourceId for service response error code
	// "InvalidResourceId".
	//
	// The resource ID is not valid. Verify that you entered the correct ID and
	// try again.
	ErrCodeInvalidResourceId = "InvalidResourceId"

	// ErrCodeInvalidResourceType for service response error code
	// "InvalidResourceType".
	//
	// The resource type is not valid. For example, if you are attempting to tag
	// an instance, the instance must be a registered, managed instance.
	ErrCodeInvalidResourceType = "InvalidResourceType"

	// ErrCodeInvalidResultAttributeException for service response error code
	// "InvalidResultAttributeException".
	//
	// The specified inventory item result attribute is not valid.
	ErrCodeInvalidResultAttributeException = "InvalidResultAttributeException"

	// ErrCodeInvalidRole for service response error code
	// "InvalidRole".
	//
	// The role name can't contain invalid characters. Also verify that you specified
	// an IAM role for notifications that includes the required trust policy. For
	// information about configuring the IAM role for Run Command notifications,
	// see Configuring Amazon SNS Notifications for Run Command (http://docs.aws.amazon.com/systems-manager/latest/userguide/rc-sns-notifications.html)
	// in the AWS Systems Manager User Guide.
	ErrCodeInvalidRole = "InvalidRole"

	// ErrCodeInvalidSchedule for service response error code
	// "InvalidSchedule".
	//
	// The schedule is invalid. Verify your cron or rate expression and try again.
	ErrCodeInvalidSchedule = "InvalidSchedule"

	// ErrCodeInvalidTarget for service response error code
	// "InvalidTarget".
	//
	// The target is not valid or does not exist. It might not be configured for
	// EC2 Systems Manager or you might not have permission to perform the operation.
	ErrCodeInvalidTarget = "InvalidTarget"

	// ErrCodeInvalidTypeNameException for service response error code
	// "InvalidTypeNameException".
	//
	// The parameter type name is not valid.
	ErrCodeInvalidTypeNameException = "InvalidTypeNameException"

	// ErrCodeInvalidUpdate for service response error code
	// "InvalidUpdate".
	//
	// The update is not valid.
	ErrCodeInvalidUpdate = "InvalidUpdate"

	// ErrCodeInvocationDoesNotExist for service response error code
	// "InvocationDoesNotExist".
	//
	// The command ID and instance ID you specified did not match any invocations.
	// Verify the command ID and the instance ID and try again.
	ErrCodeInvocationDoesNotExist = "InvocationDoesNotExist"

	// ErrCodeItemContentMismatchException for service response error code
	// "ItemContentMismatchException".
	//
	// The inventory item has invalid content.
	ErrCodeItemContentMismatchException = "ItemContentMismatchException"

	// ErrCodeItemSizeLimitExceededException for service response error code
	// "ItemSizeLimitExceededException".
	//
	// The inventory item size has exceeded the size limit.
	ErrCodeItemSizeLimitExceededException = "ItemSizeLimitExceededException"

	// ErrCodeMaxDocumentSizeExceeded for service response error code
	// "MaxDocumentSizeExceeded".
	//
	// The size limit of a document is 64 KB.
	ErrCodeMaxDocumentSizeExceeded = "MaxDocumentSizeExceeded"

	// ErrCodeOpsItemAlreadyExistsException for service response error code
	// "OpsItemAlreadyExistsException".
	//
	// The OpsItem already exists.
	ErrCodeOpsItemAlreadyExistsException = "OpsItemAlreadyExistsException"

	// ErrCodeOpsItemInvalidParameterException for service response error code
	// "OpsItemInvalidParameterException".
	//
	// A specified parameter argument isn't valid. Verify the available arguments
	// and try again.
	ErrCodeOpsItemInvalidParameterException = "OpsItemInvalidParameterException"

	// ErrCodeOpsItemLimitExceededException for service response error code
	// "OpsItemLimitExceededException".
	//
	// The request caused OpsItems to exceed one or more limits. For information
	// about OpsItem limits, see What are the resource limits for OpsCenter? (http://docs.aws.amazon.com/systems-manager/latest/userguide/OpsCenter-learn-more.html#OpsCenter-learn-more-limits).
	ErrCodeOpsItemLimitExceededException = "OpsItemLimitExceededException"

	// ErrCodeOpsItemNotFoundException for service response error code
	// "OpsItemNotFoundException".
	//
	// The specified OpsItem ID doesn't exist. Verify the ID and try again.
	ErrCodeOpsItemNotFoundException = "OpsItemNotFoundException"

	// ErrCodeParameterAlreadyExists for service response error code
	// "ParameterAlreadyExists".
	//
	// The parameter already exists. You can't create duplicate parameters.
	ErrCodeParameterAlreadyExists = "ParameterAlreadyExists"

	// ErrCodeParameterLimitExceeded for service response error code
	// "ParameterLimitExceeded".
	//
	// You have exceeded the number of parameters for this AWS account. Delete one
	// or more parameters and try again.
	ErrCodeParameterLimitExceeded = "ParameterLimitExceeded"

	// ErrCodeParameterMaxVersionLimitExceeded for service response error code
	// "ParameterMaxVersionLimitExceeded".
	//
	// The parameter exceeded the maximum number of allowed versions.
	ErrCodeParameterMaxVersionLimitExceeded = "ParameterMaxVersionLimitExceeded"

	// ErrCodeParameterNotFound for service response error code
	// "ParameterNotFound".
	//
	// The parameter could not be found. Verify the name and try again.
	ErrCodeParameterNotFound = "ParameterNotFound"

	// ErrCodeParameterPatternMismatchException for service response error code
	// "ParameterPatternMismatchException".
	//
	// The parameter name is not valid.
	ErrCodeParameterPatternMismatchException = "ParameterPatternMismatchException"

	// ErrCodeParameterVersionLabelLimitExceeded for service response error code
	// "ParameterVersionLabelLimitExceeded".
	//
	// A parameter version can have a maximum of ten labels.
	ErrCodeParameterVersionLabelLimitExceeded = "ParameterVersionLabelLimitExceeded"

	// ErrCodeParameterVersionNotFound for service response error code
	// "ParameterVersionNotFound".
	//
	// The specified parameter version was not found. Verify the parameter name
	// and version, and try again.
	ErrCodeParameterVersionNotFound = "ParameterVersionNotFound"

	// ErrCodePoliciesLimitExceededException for service response error code
	// "PoliciesLimitExceededException".
	//
	// You specified more than the maximum number of allowed policies for the parameter.
	// The maximum is 10.
	ErrCodePoliciesLimitExceededException = "PoliciesLimitExceededException"

	// ErrCodeResourceDataSyncAlreadyExistsException for service response error code
	// "ResourceDataSyncAlreadyExistsException".
	//
	// A sync configuration with the same name already exists.
	ErrCodeResourceDataSyncAlreadyExistsException = "ResourceDataSyncAlreadyExistsException"

	// ErrCodeResourceDataSyncCountExceededException for service response error code
	// "ResourceDataSyncCountExceededException".
	//
	// You have exceeded the allowed maximum sync configurations.
	ErrCodeResourceDataSyncCountExceededException = "ResourceDataSyncCountExceededException"

	// ErrCodeResourceDataSyncInvalidConfigurationException for service response error code
	// "ResourceDataSyncInvalidConfigurationException".
	//
	// The specified sync configuration is invalid.
	ErrCodeResourceDataSyncInvalidConfigurationException = "ResourceDataSyncInvalidConfigurationException"

	// ErrCodeResourceDataSyncNotFoundException for service response error code
	// "ResourceDataSyncNotFoundException".
	//
	// The specified sync name was not found.
	ErrCodeResourceDataSyncNotFoundException = "ResourceDataSyncNotFoundException"

	// ErrCodeResourceInUseException for service response error code
	// "ResourceInUseException".
	//
	// Error returned if an attempt is made to delete a patch baseline that is registered
	// for a patch group.
	ErrCodeResourceInUseException = "ResourceInUseException"

	// ErrCodeResourceLimitExceededException for service response error code
	// "ResourceLimitExceededException".
	//
	// Error returned when the caller has exceeded the default resource limits.
	// For example, too many maintenance windows or patch baselines have been created.
	//
	// For information about resource limits in Systems Manager, see AWS Systems
	// Manager Limits (http://docs.aws.amazon.com/general/latest/gr/aws_service_limits.html#limits_ssm).
	ErrCodeResourceLimitExceededException = "ResourceLimitExceededException"

	// ErrCodeServiceSettingNotFound for service response error code
	// "ServiceSettingNotFound".
	//
	// The specified service setting was not found. Either the service name or the
	// setting has not been provisioned by the AWS service team.
	ErrCodeServiceSettingNotFound = "ServiceSettingNotFound"

	// ErrCodeStatusUnchanged for service response error code
	// "StatusUnchanged".
	//
	// The updated status is the same as the current status.
	ErrCodeStatusUnchanged = "StatusUnchanged"

	// ErrCodeSubTypeCountLimitExceededException for service response error code
	// "SubTypeCountLimitExceededException".
	//
	// The sub-type count exceeded the limit for the inventory type.
	ErrCodeSubTypeCountLimitExceededException = "SubTypeCountLimitExceededException"

	// ErrCodeTargetInUseException for service response error code
	// "TargetInUseException".
	//
	// You specified the Safe option for the DeregisterTargetFromMaintenanceWindow
	// operation, but the target is still referenced in a task.
	ErrCodeTargetInUseException = "TargetInUseException"

	// ErrCodeTargetNotConnected for service response error code
	// "TargetNotConnected".
	//
	// The specified target instance for the session is not fully configured for
	// use with Session Manager. For more information, see Getting Started with
	// Session Manager (http://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started.html)
	// in the AWS Systems Manager User Guide.
	ErrCodeTargetNotConnected = "TargetNotConnected"

	// ErrCodeTooManyTagsError for service response error code
	// "TooManyTagsError".
	//
	// The Targets parameter includes too many tags. Remove one or more tags and
	// try the command again.
	ErrCodeTooManyTagsError = "TooManyTagsError"

	// ErrCodeTooManyUpdates for service response error code
	// "TooManyUpdates".
	//
	// There are concurrent updates for a resource that supports one update at a
	// time.
	ErrCodeTooManyUpdates = "TooManyUpdates"

	// ErrCodeTotalSizeLimitExceededException for service response error code
	// "TotalSizeLimitExceededException".
	//
	// The size of inventory data has exceeded the total size limit for the resource.
	ErrCodeTotalSizeLimitExceededException = "TotalSizeLimitExceededException"

	// ErrCodeUnsupportedFeatureRequiredException for service response error code
	// "UnsupportedFeatureRequiredException".
	//
	// Microsoft application patching is only available on EC2 instances and Advanced
	// Instances. To patch Microsoft applications on on-premises servers and VMs,
	// you must enable Advanced Instances. For more information, see Using the Advanced-Instances
	// Tier (http://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-managedinstances-advanced.html)
	// in the AWS Systems Manager User Guide.
	ErrCodeUnsupportedFeatureRequiredException = "UnsupportedFeatureRequiredException"

	// ErrCodeUnsupportedInventoryItemContextException for service response error code
	// "UnsupportedInventoryItemContextException".
	//
	// The Context attribute that you specified for the InventoryItem is not allowed
	// for this inventory type. You can only use the Context attribute with inventory
	// types like AWS:ComplianceItem.
	ErrCodeUnsupportedInventoryItemContextException = "UnsupportedInventoryItemContextException"

	// ErrCodeUnsupportedInventorySchemaVersionException for service response error code
	// "UnsupportedInventorySchemaVersionException".
	//
	// Inventory item type schema version has to match supported versions in the
	// service. Check output of GetInventorySchema to see the available schema version
	// for each type.
	ErrCodeUnsupportedInventorySchemaVersionException = "UnsupportedInventorySchemaVersionException"

	// ErrCodeUnsupportedOperatingSystem for service response error code
	// "UnsupportedOperatingSystem".
	//
	// The operating systems you specified is not supported, or the operation is
	// not supported for the operating system. Valid operating systems include:
	// Windows, AmazonLinux, RedhatEnterpriseLinux, and Ubuntu.
	ErrCodeUnsupportedOperatingSystem = "UnsupportedOperatingSystem"

	// ErrCodeUnsupportedParameterType for service response error code
	// "UnsupportedParameterType".
	//
	// The parameter type is not supported.
	ErrCodeUnsupportedParameterType = "UnsupportedParameterType"

	// ErrCodeUnsupportedPlatformType for service response error code
	// "UnsupportedPlatformType".
	//
	// The document does not support the platform type of the given instance ID(s).
	// For example, you sent an document for a Windows instance to a Linux instance.
	ErrCodeUnsupportedPlatformType = "UnsupportedPlatformType"
)
