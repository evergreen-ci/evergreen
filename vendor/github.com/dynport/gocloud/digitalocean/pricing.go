package digitalocean

import (
	"github.com/dynport/gocloud"
)

var megaToGiga = 1024

var Plans = []*gocloud.Plan{
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 0.7}, MemoryInMB: 512, Cores: 1, DiskInGB: 20, TrafficInTB: 1},
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 1.5}, MemoryInMB: 1 * megaToGiga, Cores: 1, DiskInGB: 30, TrafficInTB: 2},
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 3}, MemoryInMB: 2 * megaToGiga, Cores: 2, DiskInGB: 40, TrafficInTB: 3},
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 6}, MemoryInMB: 4 * megaToGiga, Cores: 2, DiskInGB: 60, TrafficInTB: 4},
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 11.9}, MemoryInMB: 8 * megaToGiga, Cores: 4, DiskInGB: 80, TrafficInTB: 5},
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 23.8}, MemoryInMB: 16 * megaToGiga, Cores: 8, DiskInGB: 160, TrafficInTB: 6},
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 47.6}, MemoryInMB: 32 * megaToGiga, Cores: 12, DiskInGB: 320, TrafficInTB: 7},
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 70.5}, MemoryInMB: 48 * megaToGiga, Cores: 16, DiskInGB: 480, TrafficInTB: 8},
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 94.1}, MemoryInMB: 64 * megaToGiga, Cores: 20, DiskInGB: 640, TrafficInTB: 9},
	{Price: &gocloud.Price{PerHour: true, Currency: gocloud.USD, Amount: 141.1}, MemoryInMB: 96 * megaToGiga, Cores: 24, DiskInGB: 960, TrafficInTB: 10},
}
