// Package do provides a thin wrapper around the DigitalOcean Kubernetes API
// for querying node pool state.
package do

import (
	"context"
	"fmt"

	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"
)

// NodePoolInfo contains the fields we care about from a DO node pool.
type NodePoolInfo struct {
	// ID is the node pool UUID.
	ID string
	// Name is the human-readable pool name.
	Name string
	// Count is the current number of nodes in the pool.
	Count int
}

// Client wraps the godo client and exposes only what the operator needs.
type Client struct {
	godo *godo.Client
}

// NewClient constructs a DO API client authenticated with the given personal
// access token.
func NewClient(token string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauthClient := oauth2.NewClient(context.Background(), ts)
	return &Client{godo: godo.NewClient(oauthClient)}
}

// GetNodePool fetches a single node pool by clusterID and poolID.
// It returns an error if the pool is not found.
func (c *Client) GetNodePool(ctx context.Context, clusterID, poolID string) (*NodePoolInfo, error) {
	pool, _, err := c.godo.Kubernetes.GetNodePool(ctx, clusterID, poolID)
	if err != nil {
		return nil, fmt.Errorf("get node pool %s/%s: %w", clusterID, poolID, err)
	}
	return &NodePoolInfo{
		ID:    pool.ID,
		Name:  pool.Name,
		Count: pool.Count,
	}, nil
}