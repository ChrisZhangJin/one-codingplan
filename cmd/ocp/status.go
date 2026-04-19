package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show upstream health and round-robin position",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

type upstreamStatus struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
	Position  bool   `json:"position"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	data, err := apiGet(flagHost + "/api/upstreams")
	if err != nil {
		return err
	}

	var upstreams []upstreamStatus
	if err := json.Unmarshal(data, &upstreams); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tHEALTHY\tPOSITION\tENABLED")
	for _, u := range upstreams {
		healthy := "yes"
		if !u.Available {
			healthy = "no"
		}
		pos := ""
		if u.Position {
			pos = ">>>"
		}
		enabled := "yes"
		if !u.Enabled {
			enabled = "no"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.Name, healthy, pos, enabled)
	}
	w.Flush()
	return nil
}
