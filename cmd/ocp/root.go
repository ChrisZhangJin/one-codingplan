package main

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	flagHost     string
	flagAdminKey string
)

var rootCmd = &cobra.Command{
	Use:   "ocp",
	Short: "one-codingplan — AI coding plan proxy aggregator",
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
