package auth

import (
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("1234567") == nil {
		t.Error("7 characters accepted")
	}
	if err := ValidatePassword("12345678"); err != nil {
		t.Errorf("8 characters rejected: %v", err)
	}
	if ValidatePassword(strings.Repeat("a", 73)) == nil {
		t.Error("73 bytes accepted (bcrypt would truncate)")
	}
}

func TestHashAndCheck(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "correct horse") || CheckPassword(h, "wrong horse") {
		t.Error("bcrypt round trip failed")
	}
}

func TestNormalizeEmail(t *testing.T) {
	got, err := NormalizeEmail("  Foo.Bar@Example.COM ")
	if err != nil || got != "foo.bar@example.com" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "foo", "foo@", "Foo <foo@example.com>", "a@localhost"} {
		if _, err := NormalizeEmail(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestDemoAllowlist(t *testing.T) {
	if _, ok := demoAccountByKey("admin"); !ok {
		t.Error("admin missing from allowlist")
	}
	for _, k := range []string{"", "Admin", "root", "admin@yozigo.test", "customer4"} {
		if _, ok := demoAccountByKey(k); ok {
			t.Errorf("%q should not be allowlisted", k)
		}
	}
	for _, a := range DemoAccounts {
		if !isReservedEmail(a.Email) {
			t.Errorf("demo email %s is not in the reserved domain", a.Email)
		}
	}
}
