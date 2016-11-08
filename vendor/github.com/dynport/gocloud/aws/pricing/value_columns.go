package pricing

import (
	"time"
)

type ValueColumns []*ValueColumn

type ValueColumn struct {
	Name   string `json:"name"`
	Rate   string `json:"rate"`
	Prices Prices `json:"prices"`
}

type Price struct {
	Name     string
	PerHour  float64
	Upfront  float64
	Currency string
	Duration time.Duration
}

func (price *Price) TotalPerHour() float64 {
	duration := price.Duration
	if duration == 0 {
		duration = OneYear
	}
	hours := duration.Hours()
	return (price.Upfront + (price.PerHour * hours)) / hours
}

func (vc *ValueColumns) ValueColumnMap() map[string]*ValueColumn {
	mapped := map[string]*ValueColumn{}
	for _, c := range *vc {
		mapped[c.Name] = c
	}
	return mapped
}

// prefix should be either yrTerm1 or yrTerm3
func (vc *ValueColumns) priceForPrefix(prefix string) *Price {
	mapped := vc.ValueColumnMap()
	if upfront := mapped[prefix]; upfront != nil {
		if upfront.Name == "linux" {
			p := &Price{}
			p.Currency = "USD"
			ok := false
			if p.PerHour, ok = upfront.Prices.USD(); ok {
				return p
			}
		}
		if perHour := mapped[prefix+"Hourly"]; perHour != nil {
			p := &Price{}
			ok := false
			if p.PerHour, ok = perHour.Prices.USD(); ok {
				p.Currency = "USD"
				if p.Upfront, ok = upfront.Prices.USD(); ok {
					return p
				}
			}
		}
	}
	return nil
}

const (
	DaysPerYear = 365
	HoursPerDay = 24
	OneYear     = time.Hour * HoursPerDay * DaysPerYear
)

type PriceList []*Price

func (list PriceList) Len() int {
	return len(list)
}

func (list PriceList) Swap(a, b int) {
	list[a], list[b] = list[b], list[a]
}

func (list PriceList) Less(a, b int) bool {
	return list[a].TotalPerHour() < list[b].TotalPerHour()
}

func (vc *ValueColumns) Prices() PriceList {
	prices := []*Price{}
	if p := vc.priceForPrefix("yrTerm1"); p != nil {
		p.Duration = 1 * OneYear
		p.Name = "1-year"
		prices = append(prices, p)
	}
	if p := vc.priceForPrefix("yrTerm3"); p != nil {
		p.Duration = 3 * OneYear
		p.Name = "3-year"
		prices = append(prices, p)
	}
	if p := vc.priceForPrefix("linux"); p != nil {
		p.Duration = 1 * OneYear
		p.Name = "on-demand"
		prices = append(prices, p)
	}
	return prices
}
