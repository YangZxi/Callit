package model

import "testing"

func TestValidateRouteWildcardRules(t *testing.T) {
	tests := []struct {
		name    string
		route   string
		wantErr bool
	}{
		{name: "支持普通路由", route: "/time", wantErr: false},
		{name: "不支持结尾非斜杠星号", route: "/time*", wantErr: true},
		{name: "支持结尾斜杠星号", route: "/tea/*", wantErr: false},
		{name: "不支持中间星号", route: "/tea/*/get", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRoute(tc.route)
			if tc.wantErr && err == nil {
				t.Fatalf("期望报错但未报错, route=%s", tc.route)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望通过但报错, route=%s err=%v", tc.route, err)
			}
		})
	}
}
