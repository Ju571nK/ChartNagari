package report

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

// Scheduler fires DailyReporter.Generate once per day at the configured KST time.
type Scheduler struct {
	reporter *DailyReporter
	cfg      appconfig.DailyReportConfig
	log      zerolog.Logger
	mu       sync.Mutex
	cancel   context.CancelFunc
}

// NewScheduler creates a Scheduler.
func NewScheduler(reporter *DailyReporter, cfg appconfig.DailyReportConfig, log zerolog.Logger) *Scheduler {
	return &Scheduler{
		reporter: reporter,
		cfg:      cfg,
		log:      log,
	}
}

// Start runs the scheduler until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	s.log.Info().
		Bool("enabled", s.cfg.Enabled).
		Str("time", s.cfg.Time).
		Str("timezone", s.cfg.Timezone).
		Msg("일일 리포트 스케줄러 시작")

	for {
		s.mu.Lock()
		cfg := s.cfg
		s.mu.Unlock()

		if !cfg.Enabled {
			// 비활성화 상태 — 1시간 후 재확인
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Hour):
				continue
			}
		}

		dur, err := nextFire(time.Now(), cfg.Time, cfg.Timezone)
		if err != nil {
			s.log.Error().Err(err).Msg("다음 발송 시간 계산 실패 — 1시간 후 재시도")
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Hour):
				continue
			}
		}

		s.log.Info().
			Str("next_fire", time.Now().Add(dur).Format(time.RFC3339)).
			Msg("다음 일일 리포트 예약")

		select {
		case <-ctx.Done():
			return
		case <-time.After(dur):
			// 현재 cfg 스냅샷으로 Generate 호출
			s.mu.Lock()
			currentCfg := s.cfg
			s.mu.Unlock()

			if currentCfg.Enabled {
				go func(c appconfig.DailyReportConfig) {
					if err := s.reporter.Generate(ctx, c, time.Now()); err != nil {
						s.log.Error().Err(err).Msg("일일 리포트 생성 실패")
					}
				}(currentCfg)
			}
		}
	}
}

// Reset stops the current timer and restarts with new config.
func (s *Scheduler) Reset(cfg appconfig.DailyReportConfig) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	s.log.Info().
		Bool("enabled", cfg.Enabled).
		Str("time", cfg.Time).
		Msg("일일 리포트 스케줄러 설정 갱신")
}

// nextFire computes the duration until the next HH:MM in the given timezone.
func nextFire(now time.Time, timeStr, tzName string) (time.Duration, error) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return 0, fmt.Errorf("타임존 로드 실패 %q: %w", tzName, err)
	}

	parts := strings.SplitN(timeStr, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("잘못된 시간 형식 %q (HH:MM 필요)", timeStr)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, fmt.Errorf("잘못된 시간 %q: %w", parts[0], err)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("잘못된 분 %q: %w", parts[1], err)
	}

	nowInTZ := now.In(loc)
	// 오늘의 목표 시각
	target := time.Date(nowInTZ.Year(), nowInTZ.Month(), nowInTZ.Day(), hour, minute, 0, 0, loc)

	// 이미 지난 경우 내일로
	if !target.After(nowInTZ) {
		target = target.AddDate(0, 0, 1)
	}

	return target.Sub(now), nil
}
