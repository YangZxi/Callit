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
	var response message.DashboardMetricsResponse
	err := s.store.Transaction(ctx, func(snapshot *db.Store) error {
		workers, err := snapshot.Worker.List(ctx, "")
		if err != nil {
			return err
		}

		summary24h, err := snapshot.WorkerLog.SummaryBetween(ctx, now.Add(-24*time.Hour), now)
		if err != nil {
			return err
		}
		summary6h, err := snapshot.WorkerLog.SummaryBetween(ctx, now.Add(-6*time.Hour), now)
		if err != nil {
			return err
		}
		workerAggregates, err := snapshot.WorkerLog.AggregateByWorkerBetween(ctx, now.Add(-24*time.Hour), now)
		if err != nil {
			return err
		}
		lastFailedWorkers, err := s.lastFailedWorkers(ctx, snapshot, workers)
		if err != nil {
			return err
		}

		response = message.DashboardMetricsResponse{
			GeneratedAt:       now,
			Workers:           buildDashboardWorkerCounts(workers),
			Summary:           buildDashboardSummary(summary24h, summary6h, len(lastFailedWorkers)),
			LastFailedWorkers: lastFailedWorkers,
			WorkerRankings:    buildDashboardWorkerRankings(workers, workerAggregates),
		}
		return nil
	})
	return response, err
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

	aggregates, err := s.store.WorkerLog.TrendBetween(ctx, workerID, startHour, finish)
	if err != nil {
		return nil, err
	}
	aggregatesByHour := make(map[int64]db.WorkerLogTrendAggregate, len(aggregates))
	for _, aggregate := range aggregates {
		aggregatesByHour[aggregate.HourUnix] = aggregate
	}

	points := make([]message.DashboardWorkerTrendPoint, dashboardTrendBuckets)
	for i := range points {
		points[i].Time = startHour.Add(time.Duration(i) * time.Hour)
		aggregate, ok := aggregatesByHour[points[i].Time.Unix()/int64(time.Hour/time.Second)]
		if !ok {
			continue
		}
		points[i].Total = aggregate.Total
		points[i].Success = aggregate.Success
		points[i].Failed = aggregate.Failed
		points[i].SuccessRate = ratePtr(aggregate.Success, aggregate.Total)
		avg := aggregate.DurationTotal / int64(aggregate.Total)
		points[i].AvgDurationMS = &avg
	}
	return points, nil
}

func (s *DashboardService) lastFailedWorkers(ctx context.Context, snapshot *db.Store, workers []model.Worker) ([]message.DashboardLastFailedWorker, error) {
	workerNames := workerNameMap(workers)
	latestLogs, err := snapshot.WorkerLog.LatestPerWorker(ctx)
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

func buildDashboardSummary(summary24h db.WorkerLogSummary, summary6h db.WorkerLogSummary, lastFailedWorkersCount int) message.DashboardSummary {
	return message.DashboardSummary{
		SuccessRate24h:         ratePtr(summary24h.Success, summary24h.Total),
		SuccessRate6h:          ratePtr(summary6h.Success, summary6h.Total),
		AvgDurationMS24h:       averageDurationPtr(summary24h.DurationTotal, summary24h.Total),
		TotalCalls24h:          summary24h.Total,
		SuccessCalls24h:        summary24h.Success,
		FailedCalls24h:         summary24h.Failed,
		TotalCalls6h:           summary6h.Total,
		FailedCalls6h:          summary6h.Failed,
		LastFailedWorkersCount: lastFailedWorkersCount,
	}
}

func buildDashboardWorkerRankings(workers []model.Worker, workerAggregates []db.WorkerLogAggregate) message.DashboardWorkerRankings {
	workerNames := workerNameMap(workers)
	aggregates := make([]dashboardWorkerAggregate, 0, len(workerAggregates))
	for _, workerAggregate := range workerAggregates {
		workerName, ok := workerNames[workerAggregate.WorkerID]
		if !ok {
			continue
		}
		aggregates = append(aggregates, dashboardWorkerAggregate{
			workerID:      workerAggregate.WorkerID,
			workerName:    workerName,
			total:         workerAggregate.Total,
			success:       workerAggregate.Success,
			failed:        workerAggregate.Failed,
			durationTotal: workerAggregate.DurationTotal,
		})
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

func workerNameMap(workers []model.Worker) map[string]string {
	names := make(map[string]string, len(workers))
	for _, worker := range workers {
		names[worker.ID] = worker.Name
	}
	return names
}

func averageDurationPtr(durationTotal int64, total int) *int64 {
	if total <= 0 {
		return nil
	}
	average := durationTotal / int64(total)
	return &average
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
