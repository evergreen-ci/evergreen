package cloudformation

type RDSDBInstance struct {
	AllocatedStorage           interface{} `json:"AllocatedStorage,omitempty"`           // String,
	AutoMinorVersionUpgrade    interface{} `json:"AutoMinorVersionUpgrade,omitempty"`    // Boolean,
	AvailabilityZone           interface{} `json:"AvailabilityZone,omitempty"`           // String,
	BackupRetentionPeriod      interface{} `json:"BackupRetentionPeriod,omitempty"`      // String,
	DBInstanceClass            interface{} `json:"DBInstanceClass,omitempty"`            // String,
	DBInstanceIdentifier       interface{} `json:"DBInstanceIdentifier,omitempty"`       // String,
	DBName                     interface{} `json:"DBName,omitempty"`                     // String,
	DBParameterGroupName       interface{} `json:"DBParameterGroupName,omitempty"`       // String,
	DBSecurityGroups           interface{} `json:"DBSecurityGroups,omitempty"`           // [[ String, ... ],
	DBSnapshotIdentifier       interface{} `json:"DBSnapshotIdentifier,omitempty"`       // String,
	DBSubnetGroupName          interface{} `json:"DBSubnetGroupName,omitempty"`          // String,
	Engine                     interface{} `json:"Engine,omitempty"`                     // String,
	EngineVersion              interface{} `json:"EngineVersion,omitempty"`              // String,
	Iops                       interface{} `json:"Iops,omitempty"`                       // Number,
	LicenseModel               interface{} `json:"LicenseModel,omitempty"`               // String,
	MasterUsername             interface{} `json:"MasterUsername,omitempty"`             // String,
	MasterUserPassword         interface{} `json:"MasterUserPassword,omitempty"`         // String,
	MultiAZ                    interface{} `json:"MultiAZ,omitempty"`                    // Boolean,
	Port                       interface{} `json:"Port,omitempty"`                       // String,
	PreferredBackupWindow      interface{} `json:"PreferredBackupWindow,omitempty"`      // String,
	PreferredMaintenanceWindow interface{} `json:"PreferredMaintenanceWindow,omitempty"` // String,
	SourceDBInstanceIdentifier interface{} `json:"SourceDBInstanceIdentifier,omitempty"` // String,
	Tags                       interface{} `json:"Tags,omitempty"`                       // [ Resource Tag, ..., ],
	VPCSecurityGroups          interface{} `json:"VPCSecurityGroups,omitempty"`          // [ String, ... ]
}

type RDSDBParameterGroup struct {
	Description interface{} `xml:"Description"` // String,
	Family      interface{} `xml:"Family"`      // String,
	Parameters  interface{} `xml:"Parameters"`  // DBParameters,
	Tags        interface{} `xml:"Tags"`        // [ Resource Tag, ... ]
}

type RDSDBSubnetGroup struct {
	DBSubnetGroupDescription interface{} `xml:"DBSubnetGroupDescription"` // String,
	SubnetIds                interface{} `xml:"SubnetIds"`                // [ String, ... ],
	Tags                     interface{} `xml:"Tags"`                     // [ Resource Tag, ... ]
}
