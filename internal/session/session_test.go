package session

import "testing"

func Test_DisplayModel_VariousFormats_NormalizesToShortForm(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  string
	}{
		{"日付付きID", "claude-sonnet-4-5-20250929", "sonnet-4-5"},
		{"短縮形", "claude-sonnet-5", "sonnet-5"},
		{"opus", "claude-opus-5", "opus-5"},
		{"空文字はそのまま", "", ""},
		{"claude-接頭辞が無くても壊れない", "sonnet-5", "sonnet-5"},
		{"日付に見えても7桁は削らない", "claude-sonnet-1234567", "sonnet-1234567"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Session{Model: c.model}
			if got := s.DisplayModel(); got != c.want {
				t.Errorf("DisplayModel() = %q, want %q", got, c.want)
			}
		})
	}
}
