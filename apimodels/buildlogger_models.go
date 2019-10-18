package apimodels

type BuildloggerInfo struct {
	BaseURL  string `json:"base_url"`
	RPCPort  string `json:"rpc_port"`
	Username string `json:"username"`
	Password string `json:"password"`
}
