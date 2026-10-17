package cmd

import (
	"testing"
	"time"
)

func TestParseHistoryDuration(t *testing.T) {
	for _, tt := range []struct {
		name, value string
		want        time.Duration
		wantErr     bool
	}{
		{name: "hours and minutes", value: "1h30m", want: 90 * time.Minute},
		{name: "days", value: "2d", want: 48 * time.Hour},
		{name: "weeks", value: "3w", want: 21 * 24 * time.Hour},
		{name: "zero", value: "0d"},
		{name: "negative days", value: "-2d", want: -48 * time.Hour},
		{name: "missing count", value: "d", wantErr: true},
		{name: "invalid count", value: "nopew", wantErr: true},
		{name: "trailing text", value: "2junkd", wantErr: true},
		{name: "fractional days", value: "1.5d", wantErr: true},
		{name: "overflow days", value: "106752d", wantErr: true},
		{name: "overflow weeks", value: "15251w", wantErr: true},
		{name: "negative overflow", value: "-106752d", wantErr: true},
		{name: "overflow count", value: "9223372036854775808d", wantErr: true},
		{name: "unknown unit", value: "2y", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHistoryDuration(tt.value)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("parseHistoryDuration(%q) = %v, %v; want %v, error=%v", tt.value, got, err, tt.want, tt.wantErr)
			}
		})
	}
}
