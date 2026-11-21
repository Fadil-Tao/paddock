package main

import "testing"

func TestParseSize(t *testing.T) {
	good := map[string]int64{
		"1048576": 1 << 20,
		"512M":    512 << 20,
		"512m":    512 << 20,
		"20G":     20 << 30,
		"20GiB":   20 << 30,
		"20GB":    20 << 30,
		"1T":      1 << 40,
		"4K":      4 << 10,
	}
	for in, want := range good {
		got, err := parseSize(in)
		if err != nil || got != want {
			t.Errorf("parseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "G", "-1G", "0", "1.5G", "10X", "99999999999T"} {
		if got, err := parseSize(in); err == nil {
			t.Errorf("parseSize(%q) = %d; want error", in, got)
		}
	}
}

func TestFormatSize(t *testing.T) {
	tests := map[int64]string{10 << 30: "10G", 512 << 20: "512M", 1536 << 20: "1536M", 100: "100B"}
	for in, want := range tests {
		if got := formatSize(in); got != want {
			t.Errorf("formatSize(%d) = %q; want %q", in, got, want)
		}
	}
}
