package main

import (
	"fmt"
	"os"

	"github.com/99designs/gqlgen/codegen/config"

	"github.com/evergreen-ci/evergreen/graphql"
)

func main() {
	fmt.Println("Generating gqlgen code...")
	// Print current working directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Failed to get current working directory: %v", err)
		os.Exit(1)
	}
	fmt.Printf("Current working directory: %s\n", cwd)
	// Load the gqlgen config from file
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		fmt.Printf("Failed to load gqlgen config: %v", err)
		// Exit with a non-zero status code to indicate failure
		os.Exit(1)
	}

	if err := cfg.LoadSchema(); err != nil {
		fmt.Println("failed to load schema: %w", err)
		os.Exit(1)

	}

	// LoadSchema again now we have everything
	if err := cfg.LoadSchema(); err != nil {
		fmt.Println("failed to load schema: %w", err)
		os.Exit(1)
	}

	if err := cfg.Init(); err != nil {
		fmt.Println("generating core failed: %w", err)
		os.Exit(1)
	}

	err = graphql.GenerateSecretFields(cfg)
	if err != nil {
		fmt.Printf("Failed to generate secret fields: %v", err)
		os.Exit(1)
	}

	// // Add your custom plugin to the gqlgen generation process
	// err = api.Generate(cfg, api.AddPlugin(graphql.NewRedactSecretsPlugin()))
	// if err != nil {
	// 	fmt.Printf("Gqlgen generation failed: %v", err)
	// 	os.Exit(1)
	// }
}
