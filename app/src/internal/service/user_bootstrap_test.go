package service_test

import (
	"regexp"
	"testing"
)

var sha256HexRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestEnsureDefaultAdmin_BootstrapAndClear(t *testing.T) {
	_, users, _, _, _, _, _ := testDB(t)

	plain, created, err := users.EnsureDefaultAdmin()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if !sha256HexRE.MatchString(plain) {
		t.Fatalf("expected 64-hex password, got %q", plain)
	}

	user, pass, pending, err := users.BootstrapCredentials()
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !pending || user != "admin" || pass != plain {
		t.Fatalf("unexpected bootstrap creds: pending=%v user=%q pass=%q", pending, user, pass)
	}

	if _, err := users.Authenticate("admin", plain); err != nil {
		t.Fatalf("auth: %v", err)
	}

	plain2, created2, err := users.EnsureDefaultAdmin()
	if err != nil {
		t.Fatalf("ensure again: %v", err)
	}
	if created2 || plain2 != "" {
		t.Fatalf("expected no recreate, got created=%v plain=%q", created2, plain2)
	}

	if err := users.ClearBootstrapCredentialsIfMatch("admin"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	_, _, pending, err = users.BootstrapCredentials()
	if err != nil {
		t.Fatalf("bootstrap after clear: %v", err)
	}
	if pending {
		t.Fatal("expected pending=false after clear")
	}

	// Password still works; only the reveal is cleared.
	if _, err := users.Authenticate("admin", plain); err != nil {
		t.Fatalf("auth after clear: %v", err)
	}
}
