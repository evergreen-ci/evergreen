package profitbricks

type Firewall struct {
	Active            string `xml:"active"`            // >false</active>
	FirewallId        string `xml:"firewallId"`        // >29f279de-027e-40e4-99f2-4f09aa086903</firewallId>
	NicId             string `xml:"nicId"`             // >910910a2-a4c5-45fe-aeff-fe7ec4d8d4d9</nicId>
	ProvisioningState string `xml:"provisioningState"` // >AVAILABLE</provisioningState>
}
