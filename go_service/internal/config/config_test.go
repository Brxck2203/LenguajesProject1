package config

import "testing"

func TestInvalidEnvironmentIsRejected(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"PORT", "70000"}, {"PORT", ""}, {"SCALA_TIMEOUT", "0s"},
		{"SHUTDOWN_TIMEOUT", "bad"}, {"ANALYSIS_CAPACITY", "0"},
		{"ANALYSIS_WORKERS", "-1"}, {"ANALYSIS_MAX_ATTEMPTS", "a"},
		{"ANALYSIS_RETRY_MAX", "1ns"},
	} {
		t.Run(tc.name+tc.value, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid setting accepted")
			}
		})
	}
}
