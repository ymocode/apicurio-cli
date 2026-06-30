package labels

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		raw     []string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "nil input yields nil",
			raw:  nil,
			want: nil,
		},
		{
			name: "empty input yields nil",
			raw:  []string{},
			want: nil,
		},
		{
			name: "repeated flags",
			raw:  []string{"bundleVersion=1.2.0", "gitTag=v1.2.0"},
			want: map[string]string{"bundleVersion": "1.2.0", "gitTag": "v1.2.0"},
		},
		{
			name: "comma-separated in one value",
			raw:  []string{"bundleVersion=1.2.0,gitTag=v1.2.0"},
			want: map[string]string{"bundleVersion": "1.2.0", "gitTag": "v1.2.0"},
		},
		{
			name: "repeated and comma forms combined",
			raw:  []string{"a=1,b=2", "c=3"},
			want: map[string]string{"a": "1", "b": "2", "c": "3"},
		},
		{
			name: "equals sign in value splits on first only",
			raw:  []string{"connection=key=value=pair"},
			want: map[string]string{"connection": "key=value=pair"},
		},
		{
			name: "empty value is allowed",
			raw:  []string{"key="},
			want: map[string]string{"key": ""},
		},
		{
			name: "surrounding whitespace is trimmed",
			raw:  []string{"  team = payments  "},
			want: map[string]string{"team": "payments"},
		},
		{
			name: "trailing and duplicate commas are ignored",
			raw:  []string{"a=1,,b=2,"},
			want: map[string]string{"a": "1", "b": "2"},
		},
		{
			name: "later value wins for duplicate key",
			raw:  []string{"a=1", "a=2"},
			want: map[string]string{"a": "2"},
		},
		{
			name:    "missing equals is an error",
			raw:     []string{"novalue"},
			wantErr: true,
		},
		{
			name:    "empty key is an error",
			raw:     []string{"=value"},
			wantErr: true,
		},
		{
			name:    "whitespace-only key is an error",
			raw:     []string{"   =value"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result: %v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse(%v) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
