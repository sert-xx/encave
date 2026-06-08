package cli

import "testing"

func TestReleasePlan(t *testing.T) {
	cases := []struct {
		name             string
		yes, interactive bool
		want             releaseMode
	}{
		{"yes auto-creates", true, false, releaseAuto},
		{"interactive prompts", false, true, releaseConfirm},
		{"non-interactive without yes skips", false, false, releaseSkip},
		{"yes wins over interactive", true, true, releaseAuto},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := releasePlan(c.yes, c.interactive); got != c.want {
				t.Errorf("releasePlan(yes=%v, interactive=%v) = %v, want %v", c.yes, c.interactive, got, c.want)
			}
		})
	}
}

func TestPushPlan(t *testing.T) {
	cases := []struct {
		name                     string
		noPush, yes, interactive bool
		want                     pushMode
	}{
		{"no-push wins over everything", true, true, true, pushSkip},
		{"yes auto-pushes", false, true, false, pushAuto},
		{"interactive prompts", false, false, true, pushConfirm},
		{"non-interactive without yes skips", false, false, false, pushSkip},
		{"no-push beats yes", true, true, false, pushSkip},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pushPlan(c.noPush, c.yes, c.interactive); got != c.want {
				t.Errorf("pushPlan(noPush=%v, yes=%v, interactive=%v) = %v, want %v",
					c.noPush, c.yes, c.interactive, got, c.want)
			}
		})
	}
}
