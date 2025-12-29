package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/grip"
)

// validationResult stores the validation outcome for a single config file
type validationResult struct {
	File   string `json:"file"`
	Passed bool   `json:"passed"`
	Errors string `json:"errors,omitempty"`
}

// validationResults stores all validation results
type validationResults struct {
	Results []validationResult `json:"results"`
	Total   int                `json:"total"`
	Passed  int                `json:"passed"`
	Failed  int                `json:"failed"`
}

func main() {
	var (
		configsDir   string
		outputFile   string
	)

	flag.StringVar(&configsDir, "configs-dir", "downloaded_configs", "Directory containing config files")
	flag.StringVar(&outputFile, "output", "", "Output file for validation results (JSON)")
	flag.Parse()

	if err := validateConfigs(configsDir, outputFile); err != nil {
		grip.Error(err)
		os.Exit(1)
	}
}

func validateConfigs(configsDir, outputFile string) error {
	configFiles, err := findConfigFiles(configsDir)
	if err != nil {
		return fmt.Errorf("finding config files: %w", err)
	}

	if len(configFiles) == 0 {
		return fmt.Errorf("no config files found in %s", configsDir)
	}

	fmt.Printf("Found %d config files to validate\n", len(configFiles))

	jobs := make(chan string, len(configFiles))
	results := make(chan validationResult, len(configFiles))

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for configFile := range jobs {
				result := validateSingleConfig(configFile)
				results <- result
			}
		}(i)
	}

	for _, configFile := range configFiles {
		jobs <- configFile
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	validationResults := validationResults{
		Results: make([]validationResult, 0, len(configFiles)),
	}

	for result := range results {
		validationResults.Results = append(validationResults.Results, result)
		validationResults.Total++
		if result.Passed {
			validationResults.Passed++
		} else {
			validationResults.Failed++
		}
	}

	fmt.Printf("\nValidation complete: %d passed, %d failed out of %d total\n",
		validationResults.Passed, validationResults.Failed, validationResults.Total)

	if outputFile != "" {
		if err := saveResults(validationResults, outputFile); err != nil {
			return fmt.Errorf("saving results: %w", err)
		}
	}

	return nil
}

func findConfigFiles(configsDir string) ([]string, error) {
	var configFiles []string

	err := filepath.Walk(configsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
			configFiles = append(configFiles, path)
		}

		return nil
	})

	return configFiles, err
}

func validateSingleConfig(configFile string) validationResult {
	result := validationResult{
		File:   configFile,
		Passed: true,
	}

	yamlBytes, err := ioutil.ReadFile(configFile)
	if err != nil {
		result.Passed = false
		result.Errors = fmt.Sprintf("failed to read file: %v", err)
		return result
	}

	var intermediate struct{}
	if err := yaml.Unmarshal(yamlBytes, &intermediate); err != nil {
		result.Passed = false
		result.Errors = fmt.Sprintf("invalid YAML: %v", err)
		return result
	}

	project := model.Project{}

	if _, err := model.LoadProjectInto(context.Background(), yamlBytes, nil, "", &project); err != nil {
		result.Passed = false
		result.Errors = fmt.Sprintf("failed to parse project: %v", err)
		return result
	}

	validationErrors := validator.CheckProjectErrors(context.Background(), &project)
	errors := validationErrors.AtLevel(validator.Error)
	if len(errors) > 0 {
		result.Passed = false
		var errorMessages []string
		for _, err := range errors {
			errorMessages = append(errorMessages, err.Message)
		}
		result.Errors = strings.Join(errorMessages, "; ")
	}

	return result
}

func saveResults(results validationResults, outputFile string) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(outputFile, data, 0644)
}
