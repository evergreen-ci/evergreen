package service

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateArtifactURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "ValidHTTPSURL",
			url:     "https://example.com/logs/foo.log",
			wantErr: false,
		},
		{
			name:    "ValidHTTPURL",
			url:     "http://example.com/artifact.txt",
			wantErr: false,
		},
		{
			name:    "AWSMetadata",
			url:     "http://169.254.169.254/latest/meta-data/iam",
			wantErr: true,
			errMsg:  "literal IP hosts are not allowed",
		},
		{
			name:    "LocalhostByName",
			url:     "http://localhost:9090/api/status/info",
			wantErr: true,
			errMsg:  "resolves to blocked address",
		},
		{
			name:    "IPv6Loopback",
			url:     "http://[::1]:8080/internal",
			wantErr: true,
			errMsg:  "literal IP hosts are not allowed",
		},
		{
			name:    "FileScheme",
			url:     "file:///etc/passwd",
			wantErr: true,
			errMsg:  "unsupported scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateArtifactURL(t.Context(), tt.url)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsBlockedIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		// Blocked IPs
		{"AWSMetadata", "169.254.169.254", true},
		{"LocalhostIPv4", "127.0.0.1", true},
		{"LocalhostIPv6", "::1", true},
		{"LinkLocalIPv4", "169.254.1.1", true},
		// Allowed IPs
		{"GoogleDNS", "8.8.8.8", false},
		{"CloudflareDNS", "1.1.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip)
			assert.Equal(t, tt.blocked, isBlockedIP(ip))
		})
	}
}

func TestSSRFDialControl(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{"PublicIPAllowed", "8.8.8.8:443", false},
		{"LoopbackBlocked", "127.0.0.1:80", true},
		{"AWSMetadataBlocked", "169.254.169.254:80", true},
		{"IPv6LoopbackBlocked", "[::1]:80", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ssrfDialControl(context.Background(), "tcp", tt.address, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "blocked address")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
