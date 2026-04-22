package admin

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"callit/internal/admin/message"
	"callit/internal/db"
	"callit/internal/model"
)

const (
	dashboardTrendBuckets = 24
	dashboardRankingLimit = 5
)

// DashboardService 负责聚合 Dashboard 所需的统计数据。
type DashboardService struct {
	store *db.Store
	now   func() time.Time
}

type dashboardWorkerAggregate struct {
	workerID      string
	workerName    string
	total         int
	success       int
	failed        int
	durationTotal int64
}

func NewDashboardService(store *db.Store) *DashboardService {
	return &DashboardService{
		store: store,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *DashboardService) Metrics(ctx context.Context) (message.DashboardMetricsResponse, error) {
	now := s.now().UTC()
	workers, err := s.store.Worker.List(ctx, "")
	if err != nil {
		return message.DashboardMetricsResponse{}, err
	}

	logs24hRaw, err := s.store.WorkerLog.ListSince(ctx, "", now.Add(-24*time.Hour))
	if err != nil {
		return message.DashboardMetricsResponse{}, err
	}
	logs24h := filterLogsBefore(logs24hRaw, now.Add(-24*time.Hour), now)
	logs6h := filterLogsBefore(logs24h, now.Add(-6*time.Hour), now)

	lastFailedWorkers, err := s.lastFailedWorkers(ctx, workers)
	if err != nil {
		return message.DashboardMetricsResponse{}, err
	}

	return message.DashboardMetricsResponse{
		GeneratedAt:       now,
		Workers:           buildDashboardWorkerCounts(workers),
		Summary:           buildDashboardSummary(logs24h, logs6h, len(lastFailedWorkers)),
		LastFailedWorkers: lastFailedWorkers,
		WorkerRankings:    buildDashboardWorkerRankings(workers, logs24h),
	}, nil
}

func (s *DashboardService) WorkerTrend(ctx context.Context, workerID string) ([]message.DashboardWorkerTrendPoint, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "all" {
		workerID = ""
	}
	if workerID != "" {
		if _, err := s.store.Worker.GetByID(ctx, workerID); err != nil {
			if errors.Is(err, sql.ErrNoRows) || db.IsNotFound(err) {
				return nil, ErrWorkerNotFound
			}
			return nil, err
		}
	}

	now := s.now().UTC()
	endHour := now.Truncate(time.Hour)
	startHour := endHour.Add(-(dashboardTrendBuckets - 1) * time.Hour)
	finish := endHour.Add(time.Hour)

	logs, err := s.store.WorkerLog.ListSince(ctx, workerID, startHour)
	if err != nil {
		return nil, err
	}

	points := make([]message.DashboardWorkerTrendPoint, dashboardTrendBuckets)
	durationSums := make([]int64, dashboardTrendBuckets)
	for i := range points {
		points[i].Time = startHour.Add(time.Duration(i) * time.Hour)
	}

	for _, log := range logs {
		createdAt := log.CreatedAt.UTC()
		if createdAt.Before(startHour) || !createdAt.Before(finish) {
			continue
		}
		index := int(createdAt.Sub(startHour) / time.Hour)
		if index < 0 || index >= len(points) {
			continue
		}

		points[index].Total++
		durationSums[index] += log.DurationMS
		if log.IsSuccess() {
			points[index].Success++
		} else {
			points[index].Failed++
		}
	}

	for i := range points {
		if points[i].Total == 0 {
			continue
		}
		points[i].SuccessRate = ratePtr(points[i].Success, points[i].Total)
		avg := durationSums[i] / int64(points[i].Total)
		points[i].AvgDurationMS = &avg
	}
	return points, nil
}

func (s *DashboardService) lastFailedWorkers(ctx context.Context, workers []model.Worker) ([]message.DashboardLastFailedWorker, error) {
	workerNames := workerNameMap(workers)
	latestLogs, err := s.store.WorkerLog.LatestPerWorker(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]message.DashboardLastFailedWorker, 0)
	for _, log := range latestLogs {
		workerName, ok := workerNames[log.WorkerID]
		if !ok {
			continue
		}
		if log.IsSuccess() {
			continue
		}
		items = append(items, message.DashboardLastFailedWorker{
			WorkerID:       log.WorkerID,
			WorkerName:     workerName,
			LastLogID:      strconv.FormatInt(log.ID, 10),
			LastRequestID:  log.RequestID,
			LastStatus:     log.Status,
			LastDurationMS: log.DurationMS,
			LastFailedAt:   log.CreatedAt,
			LastError:      log.Error,
		})
	}
	return items, nil
}

func buildDashboardWorkerCounts(workers []model.Worker) message.DashboardWorkerCounts {
	counts := message.DashboardWorkerCounts{Total: len(workers)}
	for _, worker := range workers {
		if worker.Enabled {
			counts.Enabled++
		} else {
			counts.Disabled++
		}
	}
	return counts
}

func buildDashboardSummary(logs24h []model.WorkerLog, logs6h []model.WorkerLog, lastFailedWorkersCount int) message.DashboardSummary {
	summary24h := summarizeDashboardLogs(logs24h)
	summary6h := summarizeDashboardLogs(logs6h)

	return message.DashboardSummary{
		SuccessRate24h:         summary24h.successRate,
		SuccessRate6h:          summary6h.successRate,
		AvgDurationMS24h:       summary24h.avgDurationMS,
		TotalCalls24h:          summary24h.total,
		SuccessCalls24h:        summary24h.success,
		FailedCalls24h:         summary24h.failed,
		TotalCalls6h:           summary6h.total,
		FailedCalls6h:          summary6h.failed,
		LastFailedWorkersCount: lastFailedWorkersCount,
	}
}

type dashboardLogSummary struct {
	total         int
	success       int
	failed        int
	successRate   *float64
	avgDurationMS *int64
}

func summarizeDashboardLogs(logs []model.WorkerLog) dashboardLogSummary {
	var summary dashboardLogSummary
	var durationTotal int64
	for _, log := range logs {
		summary.total++
		durationTotal += log.DurationMS
		if log.IsSuccess() {
			summary.success++
		} else {
			summary.failed++
		}
	}
	if summary.total == 0 {
		return summary
	}
	summary.successRate = ratePtr(summary.success, summary.total)
	avg := durationTotal / int64(summary.total)
	summary.avgDurationMS = &avg
	return summary
}

func buildDashboardWorkerRankings(workers []model.Worker, logs []model.WorkerLog) message.DashboardWorkerRankings {
	workerNames := workerNameMap(workers)
	aggregatesByWorker := make(map[string]*dashboardWorkerAggregate)
	for _, log := range logs {
		workerName, ok := workerNames[log.WorkerID]
		if !ok {
			continue
		}
		aggregate := aggregatesByWorker[log.WorkerID]
		if aggregate == nil {
			aggregate = &dashboardWorkerAggregate{
				workerID:   log.WorkerID,
				workerName: workerName,
			}
			aggregatesByWorker[log.WorkerID] = aggregate
		}

		aggregate.total++
		aggregate.durationTotal += log.DurationMS
		if log.IsSuccess() {
			aggregate.success++
		} else {
			aggregate.failed++
		}
	}

	aggregates := make([]dashboardWorkerAggregate, 0, len(aggregatesByWorker))
	for _, aggregate := range aggregatesByWorker {
		if aggregate.total > 0 {
			aggregates = append(aggregates, *aggregate)
		}
	}

	slowest := append([]dashboardWorkerAggregate(nil), aggregates...)
	sort.Slice(slowest, func(i, j int) bool {
		leftAvg := slowest[i].durationTotal / int64(slowest[i].total)
		rightAvg := slowest[j].durationTotal / int64(slowest[j].total)
		if leftAvg != rightAvg {
			return leftAvg > rightAvg
		}
		return slowest[i].workerName < slowest[j].workerName
	})

	leastReliable := append([]dashboardWorkerAggregate(nil), aggregates...)
	sort.Slice(leastReliable, func(i, j int) bool {
		leftRate := successRateValue(leastReliable[i].success, leastReliable[i].total)
		rightRate := successRateValue(leastReliable[j].success, leastReliable[j].total)
		if leftRate != rightRate {
			return leftRate < rightRate
		}
		if leastReliable[i].failed != leastReliable[j].failed {
			return leastReliable[i].failed > leastReliable[j].failed
		}
		return leastReliable[i].workerName < leastReliable[j].workerName
	})

	return message.DashboardWorkerRankings{
		Slowest:       buildSlowestRankItems(slowest),
		LeastReliable: buildReliableRankItems(leastReliable),
	}
}

func buildSlowestRankItems(aggregates []dashboardWorkerAggregate) []message.DashboardSlowWorkerRankItem {
	limit := min(len(aggregates), dashboardRankingLimit)
	items := make([]message.DashboardSlowWorkerRankItem, 0, limit)
	for _, aggregate := range aggregates[:limit] {
		items = append(items, message.DashboardSlowWorkerRankItem{
			WorkerID:      aggregate.workerID,
			WorkerName:    aggregate.workerName,
			Calls:         aggregate.total,
			SuccessRate:   ratePtr(aggregate.success, aggregate.total),
			AvgDurationMS: aggregate.durationTotal / int64(aggregate.total),
		})
	}
	return items
}

func buildReliableRankItems(aggregates []dashboardWorkerAggregate) []message.DashboardReliableWorkerRankItem {
	limit := min(len(aggregates), dashboardRankingLimit)
	items := make([]message.DashboardReliableWorkerRankItem, 0, limit)
	for _, aggregate := range aggregates[:limit] {
		items = append(items, message.DashboardReliableWorkerRankItem{
			WorkerID:    aggregate.workerID,
			WorkerName:  aggregate.workerName,
			Calls:       aggregate.total,
			SuccessRate: ratePtr(aggregate.success, aggregate.total),
			Failed:      aggregate.failed,
		})
	}
	return items
}

func filterLogsBefore(logs []model.WorkerLog, since time.Time, before time.Time) []model.WorkerLog {
	filtered := make([]model.WorkerLog, 0, len(logs))
	for _, log := range logs {
		createdAt := log.CreatedAt.UTC()
		if createdAt.Before(since.UTC()) || !createdAt.Before(before.UTC()) {
			continue
		}
		filtered = append(filtered, log)
	}
	return filtered
}

func workerNameMap(workers []model.Worker) map[string]string {
	names := make(map[string]string, len(workers))
	for _, worker := range workers {
		names[worker.ID] = worker.Name
	}
	return names
}

func ratePtr(success int, total int) *float64 {
	if total <= 0 {
		return nil
	}
	rate := round2(successRateValue(success, total))
	return &rate
}

func successRateValue(success int, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(success) / float64(total) * 100
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}
