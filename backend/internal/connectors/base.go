package connectors

import "context"

type Connector interface {
	Name() string
	Sync(ctx context.Context, userID string) error
}

type SyncTrigger string

const (
	SyncTriggerUnknown SyncTrigger = "unknown"
	SyncTriggerManual  SyncTrigger = "manual"
	SyncTriggerInitial SyncTrigger = "initial"
	// SyncTriggerPrefetch is the pull that runs right before the AI answers a
	// question, so the answer is about now rather than about the last cron tick.
	SyncTriggerPrefetch  SyncTrigger = "prefetch"
	SyncTriggerScheduled SyncTrigger = "scheduled"
)

// Light reports whether this run should read only the hot window. The cron tick
// and the prefetch in front of an answer both want the cheap path; a manual or
// first-ever sync is asking for the deeper one and can afford to wait.
func (t SyncTrigger) Light() bool {
	return t == SyncTriggerScheduled || t == SyncTriggerPrefetch
}

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
