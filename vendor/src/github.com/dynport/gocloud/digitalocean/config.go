package digitalocean

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
)

type Config struct {
	Accounts []*Account
}

func LoadAccount(name string) (account *Account, e error) {
	config, e := LoadConfig()
	if e != nil {
		return nil, e
	}
	return config.Account(name)
}

func LoadConfig() (config *Config, e error) {
	b, e := ioutil.ReadFile(os.Getenv("HOME") + "/.digitalocean")
	if e != nil {
		return
	}
	config = &Config{}
	e = json.Unmarshal(b, config)
	return config, e
}

func (c *Config) Account(name string) (account *Account, e error) {
	for _, account := range c.Accounts {
		if account.Name == name {
			return account, nil
		}
	}
	return nil, errors.New("no account found for " + name)
}
