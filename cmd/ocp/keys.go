package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "List access keys with limits and usage",
	RunE:  runKeys,
}

func init() {
	rootCmd.AddCommand(keysCmd)
}

type keyInfo struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Token              string     `json:"token"`
	Enabled            bool       `json:"enabled"`
	TokenBudget        int64      `json:"token_budget"`
	ExpiresAt          *time.Time `json:"expires_at"`
	RateLimitPerMinute int        `json:"rate_limit_per_minute"`
	RateLimitPerDay    int        `json:"rate_limit_per_day"`
	UsageTotalInput    int64      `json:"usage_total_input"`
	UsageTotalOutput   int64      `json:"usage_total_output"`
}

func runKeys(cmd *cobra.Command, args []string) error {
	data, err := apiGet(flagHost + "/api/keys")
	if err != nil {
		return err
	}

	var keys []keyInfo
	if err := json.Unmarshal(data, &keys); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tTOKEN\tENABLED\tBUDGET\tRPM\tRPD\tEXPIRES\tIN TOKENS\tOUT TOKENS")
	for _, k := range keys {
		shortID := k.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		enabled := "yes"
		if !k.Enabled {
			enabled = "no"
		}
		budget := "-"
		if k.TokenBudget > 0 {
			budget = fmt.Sprintf("%d", k.TokenBudget)
		}
		rpm := "-"
		if k.RateLimitPerMinute > 0 {
			rpm = fmt.Sprintf("%d", k.RateLimitPerMinute)
		}
		rpd := "-"
		if k.RateLimitPerDay > 0 {
			rpd = fmt.Sprintf("%d", k.RateLimitPerDay)
		}
		expires := "-"
		if k.ExpiresAt != nil {
			expires = k.ExpiresAt.Format("2006-01-02")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
			shortID, k.Name, k.Token, enabled, budget, rpm, rpd, expires,
			k.UsageTotalInput, k.UsageTotalOutput)
	}
	w.Flush()
	return nil
}
