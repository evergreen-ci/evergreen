package ec2

import (
	"encoding/xml"
	"log"
	"net/url"
	"strconv"
	"time"
)

type SpotPrice struct {
	InstanceType       string    `xml:"instanceType"`       // m1.small</instanceType>
	ProductDescription string    `xml:"productDescription"` // Linux/UNIX</productDescription>
	SpotPrice          float64   `xml:"spotPrice"`          // 0.287</spotPrice>
	Timestamp          time.Time `xml:"timestamp"`          // 2009-12-04T20:56:05.000Z</timestamp>
	AvailabilityZone   string    `xml:"availabilityZone"`   // us-east-1a</availabilityZone>
}

type DescribeSpotPriceHistoryResponse struct {
	SpotPrices []*SpotPrice `xml:"spotPriceHistorySet>item"`
}

const TIME_FORMAT = "2006-01-02T15:04:05.999Z"

type SpotPriceFilter struct {
	InstanceTypes       []string
	AvailabilityZones   []string
	ProductDescriptions []string
	StartTime           time.Time
	EndTime             time.Time
}

const DESC_LINUX_UNIX = "Linux/UNIX"

func (client *Client) DescribeSpotPriceHistory(filter *SpotPriceFilter) (prices []*SpotPrice, e error) {
	query := queryForAction("DescribeSpotPriceHistory")
	if filter == nil {
		filter = &SpotPriceFilter{}
	}
	values := url.Values{}
	for i, instanceType := range filter.InstanceTypes {
		values.Add("InstanceType."+strconv.Itoa(i+1), instanceType)
	}
	for i, desc := range filter.ProductDescriptions {
		values.Add("ProductDescription."+strconv.Itoa(i+1), desc)
	}

	if !filter.StartTime.IsZero() {
		values.Add("StartTime", filter.StartTime.Format(TIME_FORMAT))
	}
	if !filter.EndTime.IsZero() {
		values.Add("EndTime", filter.EndTime.Format(TIME_FORMAT))
	}
	query += "&" + values.Encode()
	log.Println(query)
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), query, nil)
	if e != nil {
		return prices, e
	}
	rsp := &DescribeSpotPriceHistoryResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return prices, e
	}
	return rsp.SpotPrices, nil
}
