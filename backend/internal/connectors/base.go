package connectors

import "context"

type Connector interface {
	Name() string
	Sync(ctx context.Context, userID string) error
}

type SyncTrigger string

const (
	SyncTriggerUnknown   SyncTrigger = "unknown"
	SyncTriggerManual    SyncTrigger = "manual"
	SyncTriggerInitial   SyncTrigger = "initial"
	SyncTriggerScheduled SyncTrigger = "scheduled"
)

type syncTriggerContextKey struct{}

func WithSyncTrigger(ctx context.Context, trigger SyncTrigger) context.Context {
	if trigger == "" {
		trigger = SyncTriggerUnknown
	}
	return context.WithValue(ctx, syncTriggerContextKey{}, trigger)
}

func GetSyncTrigger(ctx context.Context) SyncTrigger {
	if ctx == nil {
		return SyncTriggerUnknown
	}
	trigger, ok := ctx.Value(syncTriggerContextKey{}).(SyncTrigger)
	if !ok || trigger == "" {
		return SyncTriggerUnknown
	}
	return trigger
}
