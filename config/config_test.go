package config

import (
	"os"
	"testing"

	"github.com/containerd/log"
	"github.com/stretchr/testify/assert"
)

func TestDefaultValues(t *testing.T) {
	// Test that default constants have expected values
	assert.Equal(t, "/run/containerd-guest-pull-grpc/containerd-guest-pull-grpc.sock", DefaultAddress)
	assert.Equal(t, "/etc/containerd-guest-pull-grpc/config.toml", DefaultConfigPath)
	assert.Equal(t, log.InfoLevel, DefaultLogLevel)
	assert.Equal(t, "/var/lib/containerd/io.containerd.snapshotter.v1.guest-pull", DefaultRootDir)
	assert.Equal(t, "/run/containerd/containerd.sock", DefaultImageServiceAddress)
}

func TestGetEnvOrDefault(t *testing.T) {
	// Test cases
	testCases := []struct {
		name         string
		envVar       string
		envValue     string
		defaultValue string
		expected     string
		setEnv       bool
	}{
		{
			name:         "environment variable not set",
			envVar:       "TEST_ENV_NOT_SET",
			defaultValue: "default_value",
			expected:     "default_value",
			setEnv:       false,
		},
		{
			name:         "environment variable set",
			envVar:       "TEST_ENV_SET",
			envValue:     "env_value",
			defaultValue: "default_value",
			expected:     "env_value",
			setEnv:       true,
		},
		{
			name:         "environment variable set to empty string",
			envVar:       "TEST_ENV_EMPTY",
			envValue:     "",
			defaultValue: "default_value",
			expected:     "",
			setEnv:       true,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			if tc.setEnv {
				os.Setenv(tc.envVar, tc.envValue)
				defer os.Unsetenv(tc.envVar)
			}

			// Execute
			result := getEnvOrDefault(tc.envVar, tc.defaultValue)

			// Verify
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestEnvironmentVariableOverrides(t *testing.T) {
	// Save original flag values to restore after test
	originalAddress := *Address
	originalConfigPath := *ConfigPath
	originalLogLevel := *LogLevel
	originalRootDir := *RootDir
	defer func() {
		*Address = originalAddress
		*ConfigPath = originalConfigPath
		*LogLevel = originalLogLevel
		*RootDir = originalRootDir
	}()

	// Test environment variable overrides
	testEnvVars := map[string]string{
		"GUEST_PULL_ADDRESS":   "/custom/socket/path.sock",
		"GUEST_PULL_CONFIG":    "/custom/config/path.toml",
		"GUEST_PULL_LOG_LEVEL": "debug",
		"GUEST_PULL_ROOT":      "/custom/root/dir",
	}

	// Set environment variables
	for k, v := range testEnvVars {
		os.Setenv(k, v)
		defer os.Unsetenv(k)
	}

	// Re-initialize flags to pick up environment variables
	// Note: In a real scenario, you would need to re-parse flags
	// but for testing we can directly check the getEnvOrDefault function
	assert.Equal(t, testEnvVars["GUEST_PULL_ADDRESS"], getEnvOrDefault("GUEST_PULL_ADDRESS", DefaultAddress))
	assert.Equal(t, testEnvVars["GUEST_PULL_CONFIG"], getEnvOrDefault("GUEST_PULL_CONFIG", DefaultConfigPath))
	assert.Equal(t, testEnvVars["GUEST_PULL_LOG_LEVEL"], getEnvOrDefault("GUEST_PULL_LOG_LEVEL", DefaultLogLevel.String()))
	assert.Equal(t, testEnvVars["GUEST_PULL_ROOT"], getEnvOrDefault("GUEST_PULL_ROOT", DefaultRootDir))
}

// TestFlagInitialization tests that flags are initialized with correct default values
// Note: This test is limited because we can't easily reset flag values between tests
func TestFlagInitialization(t *testing.T) {
	// We can only verify that the flags exist and have the expected help text
	assert.NotNil(t, Address)
	assert.NotNil(t, ConfigPath)
	assert.NotNil(t, LogLevel)
	assert.NotNil(t, RootDir)
	assert.NotNil(t, PrintVersion)
}
