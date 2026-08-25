package session

import (
	"testing"
	"time"
)

func Test_FormatElapsed_VariousDurations_FormatsWithCoarsestUnit(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"seconds", 10 * time.Second, "10s"},
		{"just under a minute", 59 * time.Second, "59s"},
		{"minutes", 5 * time.Minute, "5m"},
		{"minutes truncates seconds", 5*time.Minute + 59*time.Second, "5m"},
		{"hours", 2 * time.Hour, "2h"},
		{"days", 3 * 24 * time.Hour, "3d"},
		{"negative clamps to zero", -5 * time.Second, "0s"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FormatElapsed(c.d); got != c.want {
				t.Errorf("FormatElapsed(%v) = %q, want %q", c.d, got, c.want)
			}
		})
	}
}
