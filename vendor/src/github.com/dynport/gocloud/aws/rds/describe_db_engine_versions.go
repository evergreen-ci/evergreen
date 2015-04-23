package rds

import (
	"encoding/xml"

	"github.com/dynport/gocloud/aws/ec2"
)

type DescribeDBEngineVersionsResponse struct {
	XMLName          xml.Name           `xml:"DescribeDBEngineVersionsResponse"`
	DBEngineVersions []*DBEngineVersion `xml:"DescribeDBEngineVersionsResult>DBEngineVersions>DBEngineVersion"`
}

type DBEngineVersion struct {
	DBParameterGroupFamily     string `xml:"DBParameterGroupFamily"`     // oracle-se1-11.2</DBParameterGroupFamily>
	Engine                     string `xml:"Engine"`                     // oracle-se1</Engine>
	DBEngineDescription        string `xml:"DBEngineDescription"`        // Oracle Database Server SE1</DBEngineDescription>
	EngineVersion              string `xml:"EngineVersion"`              // 11.2.0.2.v3</EngineVersion>
	DBEngineVersionDescription string `xml:"DBEngineVersionDescription"` // Oracle SE1 release</DBEngineVersionDescription>
}

type DescribeDBEngineVersions struct {
	DBParameterGroupFamily     string
	DefaultOnly                string
	Engine                     string
	EngineVerion               string
	MaxRecords                 int
	Marker                     string
	ListSupportedCharacterSets bool
	Filters                    []*ec2.Filter
}

const Version = "2013-05-15"

func (d *DescribeDBEngineVersions) Execute(client *Client) (*DescribeDBEngineVersionsResponse, error) {
	v := newAction("DescribeDBEngineVersions")
	e := loadValues(v, d)
	if e != nil {
		return nil, e
	}
	r := &DescribeDBEngineVersionsResponse{}
	e = client.loadResource("GET", client.Endpoint()+"?"+v.Encode(), nil, r)
	return r, e
}
