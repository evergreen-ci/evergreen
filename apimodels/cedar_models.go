package apimodels

type CedarConfig struct {
	BaseURL  string `json:"base_url"`
	RPCPort  string `json:"rpc_port"`
	Username string `json:"username"`
	APIKey   string `json:"api_key,omitempty"`
	Insecure bool   `json:"insecure"`
}

type CedarTestResultsTaskInfo struct {
	Failed bool `json:"failed"`
}
