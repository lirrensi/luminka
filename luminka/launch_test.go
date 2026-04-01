// FILE: luminka/launch_test.go
// PURPOSE: Verify parsing of Luminka-owned launch flag overrides.
// OWNS: Unit coverage for launch flag parsing and policy conflict handling.
// EXPORTS: TestParseLaunchOptions
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"strings"
	"testing"
)

func TestParseLaunchOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    launchOptions
		wantErr string
	}{
		{name: "empty args"},
		{name: "detached", args: []string{"--detached"}, want: launchOptions{RootPolicy: RootPolicyDetached}},
		{name: "portable", args: []string{"--portable"}, want: launchOptions{RootPolicy: RootPolicyPortable}},
		{name: "headless", args: []string{"--headless"}, want: launchOptions{Headless: true}},
		{name: "root policy detached", args: []string{"--root-policy", "detached"}, want: launchOptions{RootPolicy: RootPolicyDetached}},
		{name: "root path", args: []string{"--root", "notes"}, want: launchOptions{Root: "notes"}},
		{name: "conflicting flags", args: []string{"--portable", "--detached"}, wantErr: "conflicting root policy overrides"},
		{name: "unknown root policy", args: []string{"--root-policy", "invalid"}, wantErr: "unknown root policy"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLaunchOptions(tc.args)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if got := err.Error(); got == "" || !strings.Contains(got, tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("parseLaunchOptions(%v) = %#v, want %#v", tc.args, got, tc.want)
			}
		})
	}
}
