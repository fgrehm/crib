package ui

import "testing"

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		b    int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1610612736, "1.5 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatBytes(tt.b)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.b, got, tt.want)
			}
		})
	}
}
