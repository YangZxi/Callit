package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"callit/internal/db"
	"callit/internal/model"
	"callit/internal/worker/executor"

	"github.com/google/uuid"
)

type Reloader interface {
	Reload(ctx context.Context) error
}

type Manager struct {
	store   *db.Store
	invoker *executor.Service
	loc     *time.Location

	mu      sync.RWMutex
	entries []scheduledTask

	reloadCh chan struct{}
}

type scheduledTask struct {
	task     model.CronTask
	worker   model.Worker
	schedule cronExpression
}

func NewManager(store *db.Store, invoker *executor.Service, loc *time.Location) *Manager {
	if loc == nil {
		loc = time.Local
	}
	return &Manager{
		store:    store,
		invoker:  invoker,
		loc:      loc,
		reloadCh: make(chan struct{}, 1),
	}
}

func ValidateExpression(raw string) error {
	_, err := parseCronExpression(raw)
	return err
}

func (m *Manager) Reload(ctx context.Context) error {
	items, err := m.store.CronTask.ListEnabledWithWorkers(ctx)
	if err != nil {
		return err
	}

	nextEntries := make([]scheduledTask, 0, len(items))
	for _, item := range items {
		schedule, err := parseCronExpression(item.Task.Cron)
		if err != nil {
			return fmt.Errorf("解析 cron_task(%d) 表达式失败: %w", item.Task.ID, err)
		}
		nextEntries = append(nextEntries, scheduledTask{
			task:     item.Task,
			worker:   item.Worker,
			schedule: schedule,
		})
	}

	sort.Slice(nextEntries, func(i, j int) bool {
		return nextEntries[i].task.ID < nextEntries[j].task.ID
	})

	m.mu.Lock()
	m.entries = nextEntries
	m.mu.Unlock()

	select {
	case m.reloadCh <- struct{}{}:
	default:
	}
	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	if err := m.Reload(ctx); err != nil {
		return err
	}

	go m.loop(ctx)
	return nil
}

func (m *Manager) loop(ctx context.Context) {
	timer := time.NewTimer(durationUntilNextSecond(m.loc))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.reloadCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(durationUntilNextSecond(m.loc))
		case <-timer.C:
			m.runDueTasks(time.Now().In(m.loc).Truncate(time.Second))
			timer.Reset(durationUntilNextSecond(m.loc))
		}
	}
}

func (m *Manager) runDueTasks(now time.Time) {
	m.mu.RLock()
	entries := make([]scheduledTask, len(m.entries))
	copy(entries, m.entries)
	m.mu.RUnlock()

	for _, entry := range entries {
		if !entry.schedule.matches(now) {
			continue
		}

		entry := entry
		go func() {
			requestID := uuid.NewString()
			input := buildCronInput(entry.worker, requestID)
			timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Duration(entry.worker.TimeoutMS)*time.Millisecond)
			defer cancel()

			worker := entry.worker
			workerRunningTempDir, cleanup, err := executor.CreateWorkerRunningTempDir(m.invoker.WorkerRunningTempDir(), requestID)
			if err != nil {
				slog.Error("cron 创建运行时目录失败", "request_id", requestID, "worker_id", worker.ID, "err", err)
				return
			}
			defer cleanup()
			m.invoker.Execute(timeoutCtx, worker, requestID, workerRunningTempDir, input)

		}()
	}
}

func buildCronInput(worker model.Worker, requestID string) model.WorkerInput {
	return model.WorkerInput{
		Request: model.WorkerRequest{},
		Event: model.WorkerEvent{
			RequestID: requestID,
			Trigger:   model.WorkerTriggerCron,
			Runtime:   worker.Runtime,
			WorkerID:  worker.ID,
			Route:     worker.Route,
		},
	}
}

func durationUntilNextSecond(loc *time.Location) time.Duration {
	now := time.Now().In(loc)
	next := now.Truncate(time.Second).Add(time.Second)
	return next.Sub(now)
}

type cronExpression struct {
	seconds  fieldSet
	minutes  fieldSet
	hours    fieldSet
	days     fieldSet
	months   fieldSet
	weekdays fieldSet
}

func (expr cronExpression) matches(now time.Time) bool {
	weekday := int(now.Weekday())
	return expr.seconds.contains(now.Second()) &&
		expr.minutes.contains(now.Minute()) &&
		expr.hours.contains(now.Hour()) &&
		expr.days.contains(now.Day()) &&
		expr.months.contains(int(now.Month())) &&
		expr.weekdays.contains(weekday)
}

type fieldSet map[int]struct{}

func (set fieldSet) contains(value int) bool {
	_, ok := set[value]
	return ok
}

func parseCronExpression(raw string) (cronExpression, error) {
	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) != 5 && len(parts) != 6 {
		return cronExpression{}, fmt.Errorf("cron 表达式必须是 5 段或 6 段")
	}

	secondExpr := "0"
	minuteIndex := 0
	if len(parts) == 6 {
		secondExpr = parts[0]
		minuteIndex = 1
	}

	seconds, err := parseField(secondExpr, 0, 59, false)
	if err != nil {
		return cronExpression{}, fmt.Errorf("second 字段非法: %w", err)
	}
	minutes, err := parseField(parts[minuteIndex], 0, 59, false)
	if err != nil {
		return cronExpression{}, fmt.Errorf("minute 字段非法: %w", err)
	}
	hours, err := parseField(parts[minuteIndex+1], 0, 23, false)
	if err != nil {
		return cronExpression{}, fmt.Errorf("hour 字段非法: %w", err)
	}
	days, err := parseField(parts[minuteIndex+2], 1, 31, false)
	if err != nil {
		return cronExpression{}, fmt.Errorf("day 字段非法: %w", err)
	}
	months, err := parseField(parts[minuteIndex+3], 1, 12, false)
	if err != nil {
		return cronExpression{}, fmt.Errorf("month 字段非法: %w", err)
	}
	weekdays, err := parseField(parts[minuteIndex+4], 0, 7, true)
	if err != nil {
		return cronExpression{}, fmt.Errorf("weekday 字段非法: %w", err)
	}

	return cronExpression{
		seconds:  seconds,
		minutes:  minutes,
		hours:    hours,
		days:     days,
		months:   months,
		weekdays: weekdays,
	}, nil
}

func parseField(raw string, min int, max int, mapWeekday bool) (fieldSet, error) {
	result := make(fieldSet)

	segments := strings.Split(strings.TrimSpace(raw), ",")
	for _, segment := range segments {
		if err := appendSegment(result, strings.TrimSpace(segment), min, max, mapWeekday); err != nil {
			return nil, err
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("字段不能为空")
	}
	return result, nil
}

func appendSegment(target fieldSet, raw string, min int, max int, mapWeekday bool) error {
	if raw == "" {
		return fmt.Errorf("字段不能为空")
	}

	base := raw
	step := 1
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("步长格式非法")
		}
		base = strings.TrimSpace(parts[0])
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || n <= 0 {
			return fmt.Errorf("步长必须为正整数")
		}
		step = n
	}

	rangeStart := min
	rangeEnd := max
	switch {
	case base == "*" || base == "":
	case strings.Contains(base, "-"):
		parts := strings.Split(base, "-")
		if len(parts) != 2 {
			return fmt.Errorf("范围格式非法")
		}
		start, err := parseNumber(parts[0], min, max, mapWeekday)
		if err != nil {
			return err
		}
		end, err := parseNumber(parts[1], min, max, mapWeekday)
		if err != nil {
			return err
		}
		if start > end {
			return fmt.Errorf("范围起点不能大于终点")
		}
		rangeStart = start
		rangeEnd = end
	default:
		value, err := parseNumber(base, min, max, mapWeekday)
		if err != nil {
			return err
		}
		rangeStart = value
		rangeEnd = value
	}

	for value := rangeStart; value <= rangeEnd; value += step {
		mappedValue := value
		if mapWeekday && mappedValue == 7 {
			mappedValue = 0
		}
		target[mappedValue] = struct{}{}
	}
	return nil
}

func parseNumber(raw string, min int, max int, mapWeekday bool) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("数值格式非法")
	}
	if mapWeekday && value == 7 {
		return 0, nil
	}
	if value < min || value > max {
		return 0, fmt.Errorf("数值超出范围")
	}
	return value, nil
}
