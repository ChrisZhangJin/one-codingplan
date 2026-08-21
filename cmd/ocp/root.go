package main

import (
	"os"

	"github.com/spf13/cobra"
)

// version is the ocp release version. Keep it in step with the image tag in
// docker-compose.yaml when cutting a release.
const version = "0.2.2"

var (
	flagHost     string
	flagAdminKey string
)

var rootCmd = &cobra.Command{
	Use:     "ocp",
	Short:   "one-codingplan — AI coding plan proxy aggregator",
	Version: version,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagHost, "host", envOrDefault("OCP_HOST", "http://localhost:8080"), "ocp server base URL")
	rootCmd.PersistentFlags().StringVar(&flagAdminKey, "admin-key", os.Getenv("OCP_ADMIN_KEY"), "admin bearer token")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
