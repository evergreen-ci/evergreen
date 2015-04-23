package cloudformation

var AWSTemplateFormatVersion = "2010-09-09"

func NewTemplate(desc string) *Template {
	return &Template{
		AWSTemplateFormatVersion: AWSTemplateFormatVersion,
		Description:              desc,
	}
}

type Template struct {
	AWSTemplateFormatVersion string `json:"AWSTemplateFormatVersion,omitempty"`
	Description              string `json:"Description,omitempty"`

	Parameters map[string]*Parameter `json:"Parameters,omitempty"`
	Resources  Resources             `json:"Resources,omitempty"`
}

type Resources map[string]interface{}

type Resource struct {
	Type         string      `json:"Type,omitempty"`
	Properties   interface{} `json:"Properties,omitempty"`
	UpdatePolicy interface{} `json:"UpdatePolicy,omitempty"`
}

func NewResource(theType string, properties interface{}) *Resource {
	return &Resource{
		Type: theType, Properties: properties,
	}
}

type Properties struct {
	Type       string      `json:"Type,omitempty"`
	Properties *Properties `json:"Properties,omitempty"`
	MinValue   int         `json:"MinValue,omitempty"`
}

// AWS::ElasticLoadBalancing::LoadBalancer

type LoadBalancer struct {
	LoadBalancerName interface{}  `json:"LoadBalancerName,omitempty"`
	CrossZone        interface{}  `json:"CrossZone,omitempty"`
	HealthCheck      *HealthCheck `json:"HealthCheck,omitempty"`
	Listeners        []*Listener  `json:"Listeners,omitempty"`
	SecurityGroups   interface{}  `json:"SecurityGroups,omitempty"`
	Subnets          interface{}  `json:"Subnets,omitempty"`
}

type HealthCheck struct {
	HealthyThreshold   interface{} `json:"HealthyThreshold,omitempty"`
	Interval           interface{} `json:"Interval,omitempty"`
	Target             interface{} `json:"Target,omitempty"`
	Timeout            interface{} `json:"Timeout,omitempty"`
	UnhealthyThreshold interface{} `json:"UnhealthyThreshold,omitempty"`
}

type Listener struct {
	InstancePort     interface{}   `json:"InstancePort,omitempty"`
	LoadBalancerPort interface{}   `json:"LoadBalancerPort,omitempty"`
	Protocol         interface{}   `json:"Protocol,omitempty"`
	InstanceProtocol interface{}   `json:"InstanceProtocol,omitempty"`
	SSLCertificateId interface{}   `json:"SSLCertificateId,omitempty"`
	PolicyNames      []interface{} `json:"PolicyNames,omitempty"`
}

type Policy struct {
	PolicyName interface{}        `json:"PolicyName,omitempty"`
	PolicyType interface{}        `json:"PolicyType,omitempty"`
	Attributes []*PolicyAttribute `json:"Attributes,omitempty"`
}

// AWS::AutoScaling::LaunchConfiguration
type LaunchConfiguration struct {
	InstanceType        interface{}           `json:"InstanceType,omitempty"`
	ImageId             interface{}           `json:"ImageId,omitempty"`
	KeyName             interface{}           `json:"KeyName,omitempty"`
	SecurityGroups      []interface{}         `json:"SecurityGroups,omitempty"`
	UserData            interface{}           `json:"UserData,omitempty"`
	BlockDeviceMappings []*BlockDeviceMapping `json:"BlockDeviceMappings,omitempty"`
}

// AWS::AutoScaling::AutoScalingGroup",
type AutoScalingGroup struct {
	AvailabilityZones       []interface{} `json:"AvailabilityZones,omitempty"`
	Cooldown                interface{}   `json:"Cooldown,omitempty"`
	DesiredCapacity         interface{}   `json:"DesiredCapacity,omitempty"`
	HealthCheckGracePeriod  interface{}   `json:"HealthCheckGracePeriod,omitempty"`
	HealthCheckType         interface{}   `json:"HealthCheckType,omitempty"`
	LaunchConfigurationName interface{}   `json:"LaunchConfigurationName,omitempty"`
	LoadBalancerNames       []interface{} `json:"LoadBalancerNames,omitempty"`
	MaxSize                 interface{}   `json:"MaxSize,omitempty"`
	MinSize                 interface{}   `json:"MinSize,omitempty"`
}

type NotificationConfiguration struct {
	TopicARN          interface{}   `json:"TopicARN,omitempty"`
	NotificationTypes []interface{} `json:"NotificationTypes,omitempty"`
	Tags              []*Tag        `json:"Tags,omitempty"`
}

type Tag struct {
	Key               interface{} `json:"Key,omitempty"`
	Value             interface{} `json:"Value,omitempty"`
	PropagateAtLaunch interface{} `json:"PropagateAtLaunch,omitempty"`
	VPCZoneIdentifier interface{} `json:"VPCZoneIdentifier,omitempty"`
}

// AWS::CloudWatch::Alarm
type Alarm struct {
	ActionsEnabled     interface{}   `json:"ActionsEnabled,omitempty"`
	ComparisonOperator interface{}   `json:"ComparisonOperator,omitempty"`
	EvaluationPeriods  interface{}   `json:"EvaluationPeriods,omitempty"`
	MetricName         interface{}   `json:"MetricName,omitempty"`
	Namespace          interface{}   `json:"Namespace,omitempty"`
	Period             interface{}   `json:"Period,omitempty"`
	Statistic          interface{}   `json:"Statistic,omitempty"`
	Threshold          interface{}   `json:"Threshold,omitempty"`
	AlarmActions       []interface{} `json:"AlarmActions,omitempty"`
}

type BlockDeviceMapping struct {
	DeviceName interface{} `json:"DeviceName,omitempty"`
	Ebs        *Ebs        `json:"Ebs,omitempty"`
}

type Ebs struct {
	VolumeSize interface{} `json:"VolumeSize,omitempty"`
}

type PolicyAttribute struct {
	Name           interface{}   `json:"Name,omitempty"`
	Value          interface{}   `json:"Value,omitempty"`
	SecurityGroups []interface{} `json:"SecurityGroups,omitempty"`
	Subnets        interface{}   `json:"Subnets,omitempty"`
}

// AWS::EC2::SecurityGroup
type SecurityGroupProperties struct {
	GroupDescription     interface{}     `json:"GroupDescription,omitempty"`
	VpcId                interface{}     `json:"VpcId,omitempty"`
	SecurityGroupIngress []SecurityGroup `json:"SecurityGroupIngress,omitempty"`
	SecurityGroupEgress  []SecurityGroup `json:"SecurityGroupEgress,omitempty"`
}

type SecurityGroup struct {
	VpcId                interface{}          `json:"VpcId,omitempty"`
	GroupDescription     interface{}          `json:"GroupDescription,omitempty"`
	SecurityGroupEgress  []*SecurityGroupRule `json:"SecurityGroupEgress,omitempty"`
	SecurityGroupIngress []*SecurityGroupRule `json:"SecurityGroupIngress,omitempty"`
	Tags                 []*Tag               `json:"Tags,omitempty"`
}

type SecurityGroupRule struct {
	IpProtocol                 interface{} `json:"IpProtocol,omitempty"`
	FromPort                   interface{} `json:"FromPort,omitempty"`
	ToPort                     interface{} `json:"ToPort,omitempty"`
	CidrIp                     interface{} `json:"CidrIp,omitempty"`
	SourceSecurityGroupId      interface{} `json:"SourceSecurityGroupId,omitempty"`
	SourceSecurityGroupOwnerId interface{} `json:"SourceSecurityGroupOwnerId,omitempty"`
}

// AWS::RDS::DBSubnetGroup
type DBSubnetGroup struct {
	DBSubnetGroupDescription interface{} `json:"DBSubnetGroupDescription,omitempty"`
	SubnetIds                interface{} `json:"SubnetIds,omitempty"`
}

// AWS::SNS::Topic
type Topic struct {
	DisplayName string `json:"DisplayName,omitempty"`
}

// AWS::ElastiCache::CacheCluster
type CacheClusterProperties struct {
	AutoMinorVersionUpgrade    interface{}   `json:"AutoMinorVersionUpgrade,omitempty"`
	CacheNodeType              interface{}   `json:"CacheNodeType,omitempty"`
	ClusterName                interface{}   `json:"ClusterName,omitempty"`
	Engine                     interface{}   `json:"Engine,omitempty"`
	EngineVersion              interface{}   `json:"EngineVersion,omitempty"`
	NotificationTopicArn       interface{}   `json:"NotificationTopicArn,omitempty"`
	NumCacheNodes              interface{}   `json:"NumCacheNodes,omitempty"`
	PreferredAvailabilityZone  interface{}   `json:"PreferredAvailabilityZone,omitempty"`
	PreferredMaintenanceWindow interface{}   `json:"PreferredMaintenanceWindow,omitempty"`
	VpcSecurityGroupIds        []interface{} `json:"VpcSecurityGroupIds,omitempty"`
}

// AWS::RDS::DBParameterGroup
type DBParameterGroup struct {
	Description interface{} `json:"Description,omitempty"`
	Family      interface{} `json:"Family,omitempty"`
	Parameters  interface{} `json:"Parameters,omitempty"`
}

// AWS::RDS::DBInstance
type DBInstance struct {
	AllocatedStorage           interface{}   `json:"	AllocatedStorage,omitempty"`
	AutoMinorVersionUpgrade    interface{}   `json:"	AutoMinorVersionUpgrade,omitempty"`
	BackupRetentionPeriod      interface{}   `json:"	BackupRetentionPeriod,omitempty"`
	DBSubnetGroupName          interface{}   `json:"	DBSubnetGroupName,omitempty"`
	DBInstanceClass            interface{}   `json:"	DBInstanceClass,omitempty"`
	DBInstanceIdentifier       interface{}   `json:"	DBInstanceIdentifier,omitempty"`
	DBName                     interface{}   `json:"	DBName,omitempty"`
	Engine                     interface{}   `json:"	Engine,omitempty"`
	EngineVersion              interface{}   `json:"	EngineVersion,omitempty"`
	LicenseModel               interface{}   `json:"	LicenseModel,omitempty"`
	MasterUsername             interface{}   `json:"	MasterUsername,omitempty"`
	MasterUserPassword         interface{}   `json:"	MasterUserPassword,omitempty"`
	MultiAZ                    interface{}   `json:"	MultiAZ,omitempty"`
	Port                       interface{}   `json:"	Port,omitempty"`
	PreferredBackupWindow      interface{}   `json:"	PreferredBackupWindow,omitempty"`
	PreferredMaintenanceWindow interface{}   `json:"	PreferredMaintenanceWindow,omitempty"`
	Tags                       []interface{} `json:"	Tags,omitempty"`
	VPCSecurityGroups          []interface{} `json:"	VPCSecurityGroups,omitempty"`
}

type Property struct {
	Type string `json:"Type,omitempty"`
}

type Parameter struct {
	Type    string `json:"Type,omitempty"`
	Default string `json:"Default,omitempty"`
}
