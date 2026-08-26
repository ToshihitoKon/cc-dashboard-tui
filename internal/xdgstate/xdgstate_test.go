package xdgstate

import "testing"

func Test_Dir_XDGStateHomeSet_UsesIt(t *testing.T) {
	got, err := Dir("/custom/state", "/home/alice")
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	want := "/custom/state/cc-dashboard-tui"
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func Test_Dir_XDGStateHomeEmpty_FallsBackToDotLocalState(t *testing.T) {
	got, err := Dir("", "/home/alice")
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	want := "/home/alice/.local/state/cc-dashboard-tui"
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func Test_Dir_NoHomeNoXDG_ReturnsError(t *testing.T) {
	_, err := Dir("", "")
	if err == nil {
		t.Error("Dir() error = nil, want an error when both are empty")
	}
}
