package apimodels

// DataPipesConfig represents the API model for the Data-Pipes configuration.
type DataPipesConfig struct {
	Host         string `json:"host"`
	Region       string `json:"region"`
	AWSAccessKey string `json:"aws_access_key"`
	AWSSecretKey string `json:"aws_secret_key"`
	AWSToken     string `json:"aws_token"`
}
