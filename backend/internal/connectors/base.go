package connectors

import "context"

type Connector interface {
	Name() string
	Sync(ctx context.Context, userID string) error
}
