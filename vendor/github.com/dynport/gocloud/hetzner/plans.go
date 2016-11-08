package hetzner

import (
	"github.com/dynport/gocloud"
)

func NewHetznerPrice(setup, amount float64) *gocloud.Price {
	return &gocloud.Price{
		Amount:   amount,
		Setup:    setup,
		Currency: gocloud.EUR,
	}
}

var Plans = []*gocloud.Plan{
	{HyperThreading: true, Name: "EX40", Cpu: "i7-4770", Price: NewHetznerPrice(49, 49), Cores: 4, MemoryInMB: 1024 * 32, DiskInGB: 1024 * 4096},
	{HyperThreading: true, Name: "EX40-SSD", Cpu: "i7-4770", Price: NewHetznerPrice(59, 59), Cores: 4, MemoryInMB: 1024 * 32, DiskInGB: 1024 * 480},
	{HyperThreading: true, Name: "EX60", Cpu: "i7-920", Price: NewHetznerPrice(59, 0), Cores: 4, MemoryInMB: 1024 * 48, DiskInGB: 1024 * 4096},
	{HyperThreading: true, Name: "PX60", Cpu: "E3-1270v3", Price: NewHetznerPrice(69, 99), Cores: 4, MemoryInMB: 1024 * 32, DiskInGB: 1024 * 4096},
	{HyperThreading: true, Name: "PX60-SSD", Cpu: "E3-1270v3", Price: NewHetznerPrice(79, 99), Cores: 4, MemoryInMB: 1024 * 32, DiskInGB: 1024 * 480},
	{HyperThreading: true, Name: "PX70", Cpu: "E3-1270v3", Price: NewHetznerPrice(79, 99), Cores: 4, MemoryInMB: 1024 * 32, DiskInGB: 1024 * 8192},
	{HyperThreading: true, Name: "PX70-SSD", Cpu: "E3-1270v3", Price: NewHetznerPrice(99, 99), Cores: 4, MemoryInMB: 1024 * 32, DiskInGB: 1024 * 960},
	{HyperThreading: true, Name: "PX90", Cpu: "E5-1650v2", Price: NewHetznerPrice(99, 99), Cores: 6, MemoryInMB: 1024 * 64, DiskInGB: 1024 * 4096},
	{HyperThreading: true, Name: "PX90-SSD", Cpu: "E5-1650v2", Price: NewHetznerPrice(109, 109), Cores: 6, MemoryInMB: 1024 * 64, DiskInGB: 1024 * 480},
	{HyperThreading: true, Name: "PX120", Cpu: "E5-1650v2", Price: NewHetznerPrice(129, 129), Cores: 6, MemoryInMB: 1024 * 128, DiskInGB: 1024 * 4096},
	{HyperThreading: true, Name: "PX120-SSD", Cpu: "E5-1650v2", Price: NewHetznerPrice(139, 139), Cores: 6, MemoryInMB: 1024 * 128, DiskInGB: 1024 * 480},
	{HyperThreading: true, Name: "DX150", Cpu: "E5-2620", Price: NewHetznerPrice(159, 199), Cores: 6, MemoryInMB: 1024 * 64, DiskInGB: 1024 * 0},
	{HyperThreading: true, Name: "DX290", Cpu: "E5-2620", Price: NewHetznerPrice(299, 199), Cores: 12, MemoryInMB: 1024 * 128, DiskInGB: 1024 * 0},
}
