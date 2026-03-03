package admin

import "testing"

func TestMainFileGuardByRuntime(t *testing.T) {
	cases := []struct {
		name     string
		runtime  string
		filename string
		isMain   bool
	}{
		{name: "python main", runtime: "python", filename: "main.py", isMain: true},
		{name: "python diff", runtime: "python", filename: "main.diff.py", isMain: false},
		{name: "node main", runtime: "node", filename: "main.js", isMain: true},
		{name: "node diff", runtime: "node", filename: "main.diff.js", isMain: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.filename == mainFilenameByRuntime(tc.runtime)
			if got != tc.isMain {
				t.Fatalf("unexpected main file guard, runtime=%s filename=%s got=%v want=%v", tc.runtime, tc.filename, got, tc.isMain)
			}
		})
	}
}
