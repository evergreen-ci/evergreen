package thirdparty

import "fmt"

// Infrastructure/communication-related errors

type ResponseReadError struct {
	msg string
}

func (re ResponseReadError) Error() string {
	return fmt.Sprintf("API response read error: %v", re.msg)
}

type APIUnmarshalError struct {
	body, msg string
}

func (ue APIUnmarshalError) Error() string {
	return fmt.Sprintf("API response unmarshal error on %v: %v",
		ue.body, ue.msg)
}

type APIResponseError struct {
	msg string
}

func (are APIResponseError) Error() string {
	return fmt.Sprintf("API response error: %v", are.msg)
}

// Configuration-related errors

// This error should be returned when the requested remote configuration file
// can not be found.
type FileNotFoundError struct {
	filepath string
}

func (nfe FileNotFoundError) Error() string {
	return fmt.Sprintf("Requested file at %v not found", nfe.filepath)
}

func IsFileNotFound(err error) bool {
	_, ok := err.(FileNotFoundError)
	return ok
}

type FileDecodeError struct {
	Message string
}

func (f FileDecodeError) Error() string {
	return fmt.Sprintf("file decoding failed: %v", f.Message)
}

type YAMLFormatError struct {
	Message string
}

func (y YAMLFormatError) Error() string {
	return fmt.Sprintf("invalid configuration file: %v", y.Message)
}

// When attempting to access the some API using authentication, requests may
// return 404 Not Found, instead of 403 Forbidden, under certain circumstances.
// For example, see https://developer.github.com/v3/#authentication.
// This struct should be used for errors in fetching a requested remote config.
type APIRequestError struct {
	Message          string `json:"message"`
	DocumentationUrl string `json:"documentation_url"`
}

func (are APIRequestError) Error() string {
	return fmt.Sprintf("API request error: %v", are.Message)
}
