package serviceproc

import "testing"

func TestIsTongStockCommand(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"/Users/me/.local/bin/tongstock server", true},
		{"./tongstock-server", true},
		{"/Users/me/.local/bin/tongstock quote 000001", false},
		{"/usr/bin/python -m http.server 8080", false},
		{"/tmp/not-tongstock-server", false},
	}
	for _, tt := range tests {
		if got := IsTongStockCommand(tt.command); got != tt.want {
			t.Errorf("IsTongStockCommand(%q) = %v, want %v", tt.command, got, tt.want)
		}
	}
}
