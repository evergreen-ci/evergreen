package service

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateArtifactURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid HTTPS URL",
			url:     "https://example.com/logs/foo.log",
			wantErr: false,
		},
		{
			name:    "valid HTTP URL",
			url:     "http://example.com/artifact.txt",
			wantErr: false,
		},
		{
			name:    "AWS metadata",
			url:     "http://169.254.169.254/latest/meta-data/iam",
			wantErr: true,
			errMsg:  "literal IP hosts are not allowed",
		},
		{
			name:    "localhost by name",
			url:     "http://localhost:9090/api/status/info",
			wantErr: true,
			errMsg:  "resolves to blocked address",
		},
		{
			name:    "IPv6 loopback",
			url:     "http://[::1]:8080/internal",
			wantErr: true,
			errMsg:  "literal IP hosts are not allowed",
		},
		{
			name:    "file scheme",
			url:     "file:///etc/passwd",
			wantErr: true,
			errMsg:  "unsupported scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateArtifactURL(t.Context(), tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		// Blocked IPs
		{"AWS metadata", "169.254.169.254", true},
		{"localhost IPv4", "127.0.0.1", true},
		{"localhost IPv6", "::1", true},
		// Allowed IPs
		{"Google DNS", "8.8.8.8", false},
		{"Cloudflare DNS", "1.1.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			assert.NotNil(t, ip)
			result := isBlockedIP(ip)
			assert.Equal(t, tt.blocked, result)
		})
	}
}
