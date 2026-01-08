// Package linear provides a client for the Linear API.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const apiURL = "https://api.linear.app/graphql"

// Client is a Linear API client.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// Ticket represents a Linear issue/ticket.
type Ticket struct {
	ID          string   `json:"id"`
	Identifier  string   `json:"identifier"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Priority    int      `json:"priority"`
	Labels      []string `json:"labels"`
	BranchName  string   `json:"branchName"`
}

// New creates a new Linear client.
func New(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// GetTicket fetches a ticket by its identifier (e.g., "ENG-123").
func (c *Client) GetTicket(ctx context.Context, identifier string) (*Ticket, error) {
	query := `
		query GetIssue($identifier: String!) {
			issue(id: $identifier) {
				id
				identifier
				title
				description
				branchName
				priority
				state {
					name
				}
				labels {
					nodes {
						name
					}
				}
			}
		}
	`

	variables := map[string]interface{}{
		"identifier": identifier,
	}

	resp, err := c.execute(ctx, query, variables)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Issue struct {
				ID          string `json:"id"`
				Identifier  string `json:"identifier"`
				Title       string `json:"title"`
				Description string `json:"description"`
				BranchName  string `json:"branchName"`
				Priority    int    `json:"priority"`
				State       struct {
					Name string `json:"name"`
				} `json:"state"`
				Labels struct {
					Nodes []struct {
						Name string `json:"name"`
					} `json:"nodes"`
				} `json:"labels"`
			} `json:"issue"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("linear API error: %s", result.Errors[0].Message)
	}

	issue := result.Data.Issue
	labels := make([]string, len(issue.Labels.Nodes))
	for i, l := range issue.Labels.Nodes {
		labels[i] = l.Name
	}

	return &Ticket{
		ID:          issue.ID,
		Identifier:  issue.Identifier,
		Title:       issue.Title,
		Description: issue.Description,
		State:       issue.State.Name,
		Priority:    issue.Priority,
		Labels:      labels,
		BranchName:  issue.BranchName,
	}, nil
}

// execute performs a GraphQL request to Linear.
func (c *Client) execute(ctx context.Context, query string, variables map[string]interface{}) ([]byte, error) {
	body := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}


