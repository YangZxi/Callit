package executor

import "testing"

func TestSplitBridgeOutput(t *testing.T) {
	t.Run("有日志和结果", func(t *testing.T) {
		raw := "log-1\nlog-2" + bridgeOutputSeparator + `{"status":200,"body":{"ok":true}}`
		logOutput, resultOutput, err := splitBridgeOutput(raw)
		if err != nil {
			t.Fatalf("splitBridgeOutput 返回错误: %v", err)
		}
		if logOutput != "log-1\nlog-2" {
			t.Fatalf("日志拆分错误: got=%q", logOutput)
		}
		if resultOutput != `{"status":200,"body":{"ok":true}}` {
			t.Fatalf("结果拆分错误: got=%q", resultOutput)
		}
	})

	t.Run("无日志仅结果", func(t *testing.T) {
		raw := bridgeOutputSeparator + `{"body":"ok"}`
		logOutput, resultOutput, err := splitBridgeOutput(raw)
		if err != nil {
			t.Fatalf("splitBridgeOutput 返回错误: %v", err)
		}
		if logOutput != "" {
			t.Fatalf("日志应为空: got=%q", logOutput)
		}
		if resultOutput != `{"body":"ok"}` {
			t.Fatalf("结果拆分错误: got=%q", resultOutput)
		}
	})
}

func TestSplitBridgeOutputError(t *testing.T) {
	t.Run("缺少分隔符", func(t *testing.T) {
		_, _, err := splitBridgeOutput(`{"body":"ok"}`)
		if err == nil {
			t.Fatal("缺少分隔符时应返回错误")
		}
	})

	t.Run("分隔符后为空", func(t *testing.T) {
		_, _, err := splitBridgeOutput("log" + bridgeOutputSeparator + "\n \t")
		if err == nil {
			t.Fatal("结果为空时应返回错误")
		}
	})
}
