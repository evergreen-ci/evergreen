package gocloud

const USD = "USD"
const EUR = "EUR"

type Price struct {
	Amount   float64
	Currency string
	PerHour  bool
	Setup    float64
}

type Plan struct {
	Name           string
	MemoryInMB     int
	Cores          int
	DiskInGB       int
	TrafficInTB    int
	Cpu            string
	HyperThreading bool
	Price          *Price
}

func (plan *Plan) PricePerCore() *Price {
	return plan.pricePer(plan.Cores)
}

func (plan *Plan) PricePerGbRam() *Price {
	return plan.pricePer(plan.MemoryInMB / 1024.0)
}

func (plan *Plan) pricePer(value int) *Price {
	return &Price{
		Amount:   plan.Price.Amount / float64(value),
		PerHour:  plan.Price.PerHour,
		Currency: plan.Price.Currency,
	}
}
