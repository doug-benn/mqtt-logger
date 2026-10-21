package utilities

import "os"

// GetEnvOrDefault returns the environment variable value or a default if not set
func GetEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
