package model

type APISpawnHostModify struct {
	Action   APIString `json:"action"`
	HostId   APIString `json:"host_id"`
	RDPPwd   APIString `json:"rdp_pwd"`
	AddHours APIString `json:"add_hours"`
}
