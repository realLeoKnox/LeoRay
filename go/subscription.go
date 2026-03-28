package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ─── Subscription Management ────────────────────────────────────────────────

const pathSubscriptions = "data/subscriptions.json"

// Subscription represents a saved subscription source.
type Subscription struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	LastUpdated string `json:"last_updated"`
	NodeCount   int    `json:"node_count"`
}

// LoadSubscriptions reads the subscription list from disk.
func LoadSubscriptions() ([]Subscription, error) {
	data, err := os.ReadFile(pathSubscriptions)
	if err != nil {
		return []Subscription{}, nil
	}
	var subs []Subscription
	if err := json.Unmarshal(data, &subs); err != nil {
		return []Subscription{}, nil
	}
	return subs, nil
}

// SaveSubscriptions writes the subscription list to disk.
func SaveSubscriptions(subs []Subscription) error {
	b, err := json.MarshalIndent(subs, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll("data", 0755)
	return os.WriteFile(pathSubscriptions, b, 0644)
}

// subNodesPath returns the cache file for a subscription's parsed nodes.
func subNodesPath(id string) string {
	return filepath.Join("data", fmt.Sprintf("sub_%s.json", id))
}

// generateID creates a short random hex ID for a subscription.
func generateID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// AddSubscription creates a new subscription entry and fetches its nodes.
func AddSubscription(name, urlStr string) (*Subscription, []*OutboundObject, error) {
	sub := &Subscription{
		ID:   generateID(),
		Name: name,
		URL:  urlStr,
	}

	nodes, err := fetchAndCacheSubscription(sub)
	if err != nil {
		return nil, nil, err
	}

	subs, _ := LoadSubscriptions()
	subs = append(subs, *sub)
	SaveSubscriptions(subs)

	return sub, nodes, nil
}

// RefreshSubscription re-fetches a single subscription by ID.
func RefreshSubscription(id string) ([]*OutboundObject, error) {
	subs, _ := LoadSubscriptions()
	for i := range subs {
		if subs[i].ID == id {
			nodes, err := fetchAndCacheSubscription(&subs[i])
			if err != nil {
				return nil, err
			}
			SaveSubscriptions(subs)
			return nodes, nil
		}
	}
	return nil, fmt.Errorf("subscription %s not found", id)
}

// DeleteSubscription removes a subscription and its cached nodes.
func DeleteSubscription(id string) error {
	subs, _ := LoadSubscriptions()
	var filtered []Subscription
	for _, s := range subs {
		if s.ID != id {
			filtered = append(filtered, s)
		}
	}
	os.Remove(subNodesPath(id))
	return SaveSubscriptions(filtered)
}

// fetchAndCacheSubscription downloads and parses a subscription's content,
// caches the parsed nodes, and updates the subscription metadata.
func fetchAndCacheSubscription(sub *Subscription) ([]*OutboundObject, error) {
	if sub.URL == "" {
		return nil, fmt.Errorf("subscription URL is empty")
	}

	resp, err := policyHTTPClient.Get(sub.URL)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body failed: %w", err)
	}

	// Write raw content to temp file for ParseSub to read
	tmpPath := subNodesPath(sub.ID) + ".raw"
	os.MkdirAll("data", 0755)
	os.WriteFile(tmpPath, bodyBytes, 0644)
	defer os.Remove(tmpPath)

	nodes, err := ParseSub(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	// Cache parsed nodes
	nodesJSON, _ := json.MarshalIndent(nodes, "", "  ")
	os.WriteFile(subNodesPath(sub.ID), nodesJSON, 0644)

	sub.LastUpdated = time.Now().Format("2006-01-02 15:04:05")
	sub.NodeCount = len(nodes)

	return nodes, nil
}

// MergeAllSubscriptionNodes loads and merges nodes from all subscriptions.
func MergeAllSubscriptionNodes() []*OutboundObject {
	subs, _ := LoadSubscriptions()
	var allNodes []*OutboundObject
	seen := map[string]bool{}

	for _, sub := range subs {
		data, err := os.ReadFile(subNodesPath(sub.ID))
		if err != nil {
			continue
		}
		var nodes []*OutboundObject
		if json.Unmarshal(data, &nodes) != nil {
			continue
		}
		for _, n := range nodes {
			if !seen[n.Tag] {
				seen[n.Tag] = true
				allNodes = append(allNodes, n)
			}
		}
	}

	return allNodes
}

// RefreshAllSubscriptions re-fetches all subscriptions and returns merged nodes.
func RefreshAllSubscriptions() ([]*OutboundObject, error) {
	subs, _ := LoadSubscriptions()
	for i := range subs {
		fetchAndCacheSubscription(&subs[i])
	}
	SaveSubscriptions(subs)
	return MergeAllSubscriptionNodes(), nil
}
