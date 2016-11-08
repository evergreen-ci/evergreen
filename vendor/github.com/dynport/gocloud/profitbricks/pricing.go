package profitbricks

type Pricing struct {
	CpuPerHour        float64
	GbRamPerHour      float64
	GbStoragePerMonth float64
}

var (
	HoursPerMonth float64 = 24 * 30

	DE = &Pricing{
		CpuPerHour:        2,
		GbRamPerHour:      0.45,
		GbStoragePerMonth: 9,
	}

	US = &Pricing{
		CpuPerHour:        2.5,
		GbRamPerHour:      0.75,
		GbStoragePerMonth: 9,
	}
)

type Price struct {
	Cpu     float64
	Ram     float64
	Storage float64
}

func (price *Price) TotalPrice() float64 {
	return price.Cpu + price.Ram + price.Storage
}

func (pricing *Pricing) Calculate(cpus int, ram int, storage int) *Price {
	return &Price{
		Cpu:     float64(cpus) * HoursPerMonth * pricing.CpuPerHour,
		Ram:     float64(ram) * HoursPerMonth * pricing.GbRamPerHour,
		Storage: float64(storage) * pricing.GbStoragePerMonth,
	}
}
