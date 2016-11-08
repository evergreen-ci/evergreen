package cloudformation

func (c *Client) DeleteStack(name string) error {
	return c.loadCloudFormationResource("DeleteStack", Values{"StackName": name}, nil)
}
