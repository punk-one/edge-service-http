package logging

import "testing"

func TestNewSupportsPointerConfig(t *testing.T) {
	rotating := newRotatingLogger(*normalizeConfigForTest(t, &Config{
		File:       "/tmp/runtime.log",
		MaxFiles:   6,
		MaxBackups: 2,
	}))

	if rotating.MaxBackups != 6 {
		t.Fatalf("MaxBackups = %d, want 6", rotating.MaxBackups)
	}
}

func TestNewRotatingLoggerUsesMaxFilesForRetentionCount(t *testing.T) {
	rotating := newRotatingLogger(Config{
		File:       "/tmp/runtime.log",
		MaxSize:    5,
		MaxFiles:   9,
		MaxBackups: 2,
		Compress:   true,
	})

	if rotating.MaxBackups != 9 {
		t.Fatalf("MaxBackups = %d, want 9", rotating.MaxBackups)
	}
	if rotating.MaxAge != 0 {
		t.Fatalf("MaxAge = %d, want 0 (MaxFiles is count, not days)", rotating.MaxAge)
	}
}

func TestNewRotatingLoggerFallsBackToMaxBackupsWhenMaxFilesUnset(t *testing.T) {
	rotating := newRotatingLogger(Config{
		File:       "/tmp/runtime.log",
		MaxBackups: 4,
	})

	if rotating.MaxBackups != 4 {
		t.Fatalf("MaxBackups = %d, want 4", rotating.MaxBackups)
	}
}

func normalizeConfigForTest(t *testing.T, input any) *Config {
	t.Helper()

	cfg := normalizeConfig(input, nil)
	return &cfg
}
