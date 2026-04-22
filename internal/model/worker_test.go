package model

import "testing"

func TestValidateRouteRejectsRootWildcard(t *testing.T) {
	err := ValidateRoute("/*")
	if err == nil {
		t.Fatal("/* 应被视为非法路由")
	}
	if err.Error() != "route 不能使用泛根路径 /*" {
		t.Fatalf("/* 校验错误信息不正确: %v", err)
	}
}

func TestWorkerLogIsSuccess(t *testing.T) {
	tests := []struct {
		name string
		log  WorkerLog
		want bool
	}{
		{name: "2xx 无错误为成功", log: WorkerLog{Status: 200}, want: true},
		{name: "3xx 无错误为成功", log: WorkerLog{Status: 302}, want: true},
		{name: "4xx 为失败", log: WorkerLog{Status: 404}, want: false},
		{name: "5xx 为失败", log: WorkerLog{Status: 500}, want: false},
		{name: "有错误信息为失败", log: WorkerLog{Status: 200, Error: "脚本失败"}, want: false},
		{name: "非法低状态码为失败", log: WorkerLog{Status: 0}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.log.IsSuccess(); got != tc.want {
				t.Fatalf("成功状态判断不正确，got=%v want=%v log=%#v", got, tc.want, tc.log)
			}
		})
	}
}
