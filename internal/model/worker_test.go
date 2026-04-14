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
