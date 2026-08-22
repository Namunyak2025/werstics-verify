package config

import "testing"

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("WERSTICS_VERIFY_DATABASE_URL", "")
	t.Setenv("WERSTICS_VERIFY_ADDR", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected missing database URL to fail")
	}
}

func TestLoadUsesDefaultAddress(t *testing.T) {
	t.Setenv(
		"WERSTICS_VERIFY_DATABASE_URL",
		"postgres://werstics_verify:secret@localhost:5432/werstics_verify",
	)
	t.Setenv("WERSTICS_VERIFY_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Addr != ":8080" {
		t.Fatalf(
			"expected default address :8080, got %q",
			cfg.Addr,
		)
	}
}

func TestLoadAcceptsPostgreSQLURL(t *testing.T) {
	t.Setenv(
		"WERSTICS_VERIFY_DATABASE_URL",
		"postgresql://user:secret@db.example:5432/werstics_verify",
	)
	t.Setenv("WERSTICS_VERIFY_ADDR", "127.0.0.1:9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Addr != "127.0.0.1:9090" {
		t.Fatalf("unexpected address %q", cfg.Addr)
	}
}

func TestLoadRejectsInvalidDatabaseScheme(t *testing.T) {
	t.Setenv(
		"WERSTICS_VERIFY_DATABASE_URL",
		"mysql://user:secret@localhost:3306/werstics_verify",
	)

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid database scheme to fail")
	}
}

func TestLoadRejectsDatabaseURLWithoutUsername(t *testing.T) {
	t.Setenv(
		"WERSTICS_VERIFY_DATABASE_URL",
		"postgres://localhost:5432/werstics_verify",
	)

	if _, err := Load(); err == nil {
		t.Fatal("expected missing database username to fail")
	}
}

func TestLoadRejectsDatabaseURLWithoutHost(t *testing.T) {
	t.Setenv(
		"WERSTICS_VERIFY_DATABASE_URL",
		"postgres:///werstics_verify",
	)

	if _, err := Load(); err == nil {
		t.Fatal("expected missing database host to fail")
	}
}

func TestLoadReadsSimulatorSecret(t *testing.T) {
	t.Setenv(
		"WERSTICS_VERIFY_DATABASE_URL",
		"postgres://user:secret@localhost:5432/werstics_verify",
	)
	t.Setenv("WERSTICS_VERIFY_ADDR", "")
	t.Setenv("WERSTICS_VERIFY_SIMULATOR_SECRET", "simulator-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.SimulatorSecret != "simulator-secret" {
		t.Fatalf(
			"expected simulator secret to be loaded, got %q",
			cfg.SimulatorSecret,
		)
	}
}
