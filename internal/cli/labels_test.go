package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/ymocode/apicurio-client/internal/config"
)

func TestGuardLabelsAPIVersion(t *testing.T) {
	labels := map[string]string{"bundleVersion": "1.2.0"}

	tests := []struct {
		name    string
		apiVer  config.APIVersion
		labels  map[string]string
		wantErr bool
	}{
		{name: "v3 with labels", apiVer: config.APIVersionV3, labels: labels},
		{name: "v2 with labels", apiVer: config.APIVersionV2, labels: labels, wantErr: true},
		{name: "ccompat with labels", apiVer: config.APIVersionCCOMPAT, labels: labels, wantErr: true},
		{name: "v2 without labels", apiVer: config.APIVersionV2, labels: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := guardLabelsAPIVersion(&config.Config{APIVersion: tc.apiVer}, tc.labels)
			if (err != nil) != tc.wantErr {
				t.Fatalf("guardLabelsAPIVersion(%s) error = %v, wantErr = %v", tc.apiVer, err, tc.wantErr)
			}
		})
	}
}

func TestParseLabelFlags_MergesLabelAndLabels(t *testing.T) {
	cmd := &cobra.Command{}
	addLabelFlags(cmd)
	if err := cmd.Flags().Parse([]string{
		"--labels", "bundleVersion=1.2.0,gitTag=v1.2.0",
		"--label", "gitSha=abc1234",
	}); err != nil {
		t.Fatalf("flag parse: %v", err)
	}

	got, err := parseLabelFlags(cmd)
	if err != nil {
		t.Fatalf("parseLabelFlags: %v", err)
	}

	want := map[string]string{"bundleVersion": "1.2.0", "gitTag": "v1.2.0", "gitSha": "abc1234"}
	if len(got) != len(want) {
		t.Fatalf("parsed labels = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("label %q = %q, want %q", k, got[k], v)
		}
	}
}
