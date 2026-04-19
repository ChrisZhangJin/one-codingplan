package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Force rotate to the next available upstream",
	RunE:  runNext,
}

func init() {
	rootCmd.AddCommand(nextCmd)
}

func runNext(cmd *cobra.Command, args []string) error {
	data, err := apiPost(flagHost + "/api/upstreams/rotate")
	if err != nil {
		return err
	}

	var resp struct {
		Upstream string `json:"upstream"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Rotated to: %s\n", resp.Upstream)
	return nil
}
