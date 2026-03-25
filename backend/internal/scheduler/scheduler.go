package scheduler

import (
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
)

type Scheduler struct {
	cron   *cron.Cron
	logger zerolog.Logger
}

func New(logger zerolog.Logger) *Scheduler {
	return &Scheduler{
		cron:   cron.New(cron.WithSeconds()),
		logger: logger.With().Str("component", "scheduler").Logger(),
	}
}

func (s *Scheduler) AddJob(spec, name string, fn func()) error {
	_, err := s.cron.AddFunc(spec, func() {
		start := time.Now()
		s.logger.Info().Str("job", name).Msg("job started")
		fn()
		s.logger.Info().Str("job", name).Dur("duration", time.Since(start)).Msg("job finished")
	})
	if err != nil {
		return err
	}
	s.logger.Debug().Str("job", name).Str("spec", spec).Msg("job registered")
	return nil
}

func (s *Scheduler) Start() {
	s.cron.Start()
	s.logger.Info().Msg("scheduler started")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.logger.Info().Msg("scheduler stopped")
}
