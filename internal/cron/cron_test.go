package cron

import (
	"testing"
	"time"

	"callit/internal/model"
)

func TestParseCronExpression(t *testing.T) {
	schedule, err := parseCronExpression("*/15 9-18 * * 1-5")
	if err != nil {
		t.Fatalf("解析合法 cron 表达式失败: %v", err)
	}

	matchTime := time.Date(2026, time.March, 20, 9, 30, 0, 0, time.Local)
	if !schedule.matches(matchTime) {
		t.Fatalf("cron 表达式应命中工作日 09:30")
	}

	notMatchTime := time.Date(2026, time.March, 21, 9, 30, 0, 0, time.Local)
	if schedule.matches(notMatchTime) {
		t.Fatalf("cron 表达式不应命中周六 09:30")
	}
}

func TestParseCronExpressionWithSeconds(t *testing.T) {
	schedule, err := parseCronExpression("*/30 * * * * *")
	if err != nil {
		t.Fatalf("解析 6 段 cron 表达式失败: %v", err)
	}

	matchTime := time.Date(2026, time.March, 19, 13, 30, 30, 0, time.Local)
	if !schedule.matches(matchTime) {
		t.Fatalf("cron 表达式应命中 13:30:30")
	}

	notMatchTime := time.Date(2026, time.March, 19, 13, 30, 45, 0, time.Local)
	if schedule.matches(notMatchTime) {
		t.Fatalf("cron 表达式不应命中 13:30:45")
	}
}

func TestParseCronExpressionFiveFieldsDefaultsSecondZero(t *testing.T) {
	schedule, err := parseCronExpression("30 * * * *")
	if err != nil {
		t.Fatalf("解析 5 段 cron 表达式失败: %v", err)
	}

	matchTime := time.Date(2026, time.March, 19, 13, 30, 0, 0, time.Local)
	if !schedule.matches(matchTime) {
		t.Fatalf("5 段 cron 表达式应命中 13:30:00")
	}

	notMatchTime := time.Date(2026, time.March, 19, 13, 30, 30, 0, time.Local)
	if schedule.matches(notMatchTime) {
		t.Fatalf("5 段 cron 表达式不应命中 13:30:30")
	}
}

func TestParseCronExpressionRejectsInvalid(t *testing.T) {
	for _, expr := range []string{
		"",
		"* * * *",
		"* * * * * * *",
		"61 * * * *",
		"61 * * * * *",
		"* 24 * * *",
		"* * 0 * *",
		"* * * 13 *",
		"* * * * 8",
		"*/0 * * * *",
	} {
		if _, err := parseCronExpression(expr); err == nil {
			t.Fatalf("非法 cron 表达式应返回错误: %q", expr)
		}
	}
}

func TestBuildCronInput(t *testing.T) {
	input := buildCronInput(model.Worker{
		ID:      "worker-cron",
		Runtime: "python",
		Route:   "/cron/demo",
	}, "req-cron-1")

	if input.Event.Trigger != model.WorkerTriggerCron {
		t.Fatalf("cron 输入 trigger 不正确: %#v", input.Event)
	}
	if input.Request.Method != "" || input.Request.URI != "" || input.Request.URL != "" || len(input.Request.Params) != 0 || len(input.Request.Headers) != 0 || input.Request.BodyStr != "" || input.Request.Body != nil {
		t.Fatalf("cron 输入 request 应为空对象: %#v", input.Request)
	}
}
