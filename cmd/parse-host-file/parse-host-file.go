package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

type host struct {
	DNS string `json:"dns_name"`
	ID  string `json:"instance_id"`
}

func main() {
	var hostFile string
	flag.StringVar(&hostFile, "file", "", "path to the file containing spawned host info")
	flag.Parse()

	if hostFile == "" {
		fmt.Println("no host file provided")
		os.Exit(1)
	}

	f, err := os.Open(hostFile)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	bytes, err := io.ReadAll(f)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	var hosts []host
	err = json.Unmarshal(bytes, &hosts)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	if len(hosts) < 1 {
		fmt.Println("no hosts listed in file")
		os.Exit(1)
	}

	data := []byte(fmt.Sprintf("docker_host: %s", hosts[0].DNS))
	err = os.WriteFile("bin/expansions.yml", data, 0644)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
