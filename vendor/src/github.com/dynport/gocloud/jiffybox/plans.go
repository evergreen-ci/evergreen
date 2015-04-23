package jiffybox

type Plan struct {
	Id                 int     `json:"id"`                 //22,
	Name               string  `json:"name"`               //"CloudLevel 3",
	DiskSizeInMB       int     `json:"diskSizeInMB"`       //307200,
	RamInMB            int     `json:"ramInMB"`            //8192,
	PricePerHour       float64 `json:"pricePerHour"`       //0.07,
	PricePerHourFrozen float64 `json:"pricePerHourFrozen"` //0.02,
	Cpus               int     `json:"cpus"`               //6
}

type PlansResponse struct {
	Messages []string         `json:"messages"`
	PlansMap map[string]*Plan `json:"result"`
}

type PlanResponse struct {
	Messages []string `json:"messages"`
	Plan     *Plan    `json:"result"`
}

func (rsp *PlansResponse) Plans() (plans []*Plan) {
	for _, plan := range rsp.PlansMap {
		plans = append(plans, plan)
	}
	return plans
}

func (client *Client) Plans() (plans []*Plan, e error) {
	rsp := &PlansResponse{}
	e = client.LoadResource("plans", rsp)
	if e != nil {
		return plans, e
	}
	return rsp.Plans(), e
}

func (client *Client) Plan(id string) (plan *Plan, e error) {
	rsp := &PlanResponse{}
	e = client.LoadResource("plans/"+id, rsp)
	if e != nil {
		return plan, e
	}
	return rsp.Plan, nil
}
