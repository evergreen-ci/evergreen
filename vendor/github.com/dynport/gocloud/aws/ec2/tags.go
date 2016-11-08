package ec2

type DescribeTagsResponse struct {
	Tags []*Tag `xml:"tagSet>item"`
}
