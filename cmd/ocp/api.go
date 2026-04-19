package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// apiGet performs a GET request with admin auth and returns the response body.
// On connection error (D-08): prints to stderr and exits 1.
// On non-200 response (D-09): prints error message to stderr and exits 1.
func apiGet(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+flagAdminKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot reach ocp server at %s -- is it running?\n", flagHost)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", errResp.Error)
		} else {
			fmt.Fprintf(os.Stderr, "Error: server returned %d\n", resp.StatusCode)
		}
		os.Exit(1)
	}
	return body, nil
}

// apiPost performs a POST request with admin auth and returns the response body.
// On connection error (D-08): prints to stderr and exits 1.
// On non-200 response (D-09): prints error message to stderr and exits 1.
func apiPost(url string) ([]byte, error) {
	req, err := http.NewRequest("POST", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+flagAdminKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot reach ocp server at %s -- is it running?\n", flagHost)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", errResp.Error)
		} else {
			fmt.Fprintf(os.Stderr, "Error: server returned %d\n", resp.StatusCode)
		}
		os.Exit(1)
	}
	return body, nil
}
