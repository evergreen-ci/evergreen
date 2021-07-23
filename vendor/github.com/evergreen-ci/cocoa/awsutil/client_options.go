package awsutil

import (
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// ClientOptions represent AWS client options such as authentication and making
// requests.
type ClientOptions struct {
	// Creds are the client credentials, which may be used to either connect to
	// the AWS API directly, or authenticate to STS to retrieve temporary
	// credentials to access the API (if Role is specified).
	Creds *credentials.Credentials
	// Role is the STS role that should be used to perform authorized actions.
	// If specified, Creds will be used to retrieve temporary credentials from
	// STS.
	Role *string
	// Region is the geographical region where API calls should be made.
	Region *string
	// RetryOpts sets the retry policy for API requests.
	RetryOpts *utility.RetryOptions
	// HTTPClient is the HTTP client to use to make requests.
	HTTPClient *http.Client

	stsSession *session.Session
	stsCreds   *credentials.Credentials

	session *session.Session

	ownsHTTPClient bool
}

// NewClientOptions returns new unconfigured client options.
func NewClientOptions() *ClientOptions {
	return &ClientOptions{}
}

// SetCredentials sets the client's credentials.
func (o *ClientOptions) SetCredentials(creds *credentials.Credentials) *ClientOptions {
	o.Creds = creds
	return o
}

// SetRole sets the client's role to assume.
func (o *ClientOptions) SetRole(role string) *ClientOptions {
	o.Role = &role
	return o
}

// SetRegion sets the client's geographical region.
func (o *ClientOptions) SetRegion(region string) *ClientOptions {
	o.Region = &region
	return o
}

// SetRetryOptions sets the client's retry options.
func (o *ClientOptions) SetRetryOptions(opts utility.RetryOptions) *ClientOptions {
	o.RetryOpts = &opts
	return o
}

// SetHTTPClient sets the HTTP client to use.
func (o *ClientOptions) SetHTTPClient(hc *http.Client) *ClientOptions {
	o.HTTPClient = hc
	return o
}

// Validate checks that all required fields are given and sets defaults for
// unspecified options.
func (o *ClientOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(o.Region == nil, "must provide geographical region")
	catcher.NewWhen(o.Role == nil && o.Creds == nil, "must provide either explicit credentials, role to assume, or both")

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	if o.HTTPClient == nil {
		o.HTTPClient = utility.GetHTTPClient()
		o.ownsHTTPClient = true
	}

	if o.RetryOpts == nil {
		o.RetryOpts = &utility.RetryOptions{}
	}
	o.RetryOpts.Validate()

	return nil
}

// GetCredentials retrieves the appropriate credentials to use for the client.
func (o *ClientOptions) GetCredentials() (*credentials.Credentials, error) {
	if o.Role == nil && o.Creds == nil {
		return nil, errors.New("cannot get client credentials when neither explicit credentials are given, nor the role to assume is given")
	}
	if o.Role == nil {
		return o.Creds, nil
	}

	if o.stsCreds != nil {
		return o.stsCreds, nil
	}

	if o.stsSession == nil {
		sess, err := session.NewSession(&aws.Config{
			HTTPClient:  o.HTTPClient,
			Region:      o.Region,
			Credentials: o.Creds,
		})
		if err != nil {
			return nil, errors.Wrap(err, "creating session")
		}

		o.stsSession = sess
	}

	o.stsCreds = stscreds.NewCredentials(o.stsSession, *o.Role)

	return o.stsCreds, nil
}

// GetSession gets the authenticated session to perform authorized API actions.
func (o *ClientOptions) GetSession() (*session.Session, error) {
	if o.session != nil {
		return o.session, nil
	}

	creds, err := o.GetCredentials()
	if err != nil {
		return nil, errors.Wrap(err, "getting credentials")
	}

	sess, err := session.NewSession(&aws.Config{
		HTTPClient:  o.HTTPClient,
		Region:      o.Region,
		Credentials: creds,
	})
	if err != nil {
		return nil, errors.Wrap(err, "creating session")
	}

	o.session = sess

	return o.session, nil
}

// Close cleans up the HTTP client if it is owned by this client.
func (o *ClientOptions) Close() {
	if o.ownsHTTPClient {
		utility.PutHTTPClient(o.HTTPClient)
	}
}
