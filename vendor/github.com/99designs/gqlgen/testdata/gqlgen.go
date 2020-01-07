package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/99designs/gqlgen/graphql"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin/stubgen"
)

func main() {
	stub := flag.String("stub", "", "name of stub file to generate")
	flag.Parse()

	log.SetOutput(ioutil.Discard)

	start := graphql.Now()

	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config", err.Error())
		os.Exit(2)
	}

	var options []api.Option
	if *stub != "" {
		options = append(options, api.AddPlugin(stubgen.New(*stub, "Stub")))
	}

	err = api.Generate(cfg, options...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}

	fmt.Printf("Generated %s in %4.2fs\n", cfg.Exec.ImportPath(), time.Since(start).Seconds())
}
