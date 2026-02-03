package config

import (
	"testing"
)

func BenchmarkGetFilteredConfigEntries(b *testing.B) {
	cfg := &Config{
		Entries: make([]Entry, 100),
	}

	// Populate with test data
	for i := range cfg.Entries {
		cfg.Entries[i] = Entry{
			Name:   "test",
			Backup: "./test",
		}
	}

	ctx := &FilterContext{
		OS:       "linux",
		Distro:   "arch",
		Hostname: "localhost",
		User:     "user",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cfg.GetFilteredConfigEntries(ctx)
	}
}
