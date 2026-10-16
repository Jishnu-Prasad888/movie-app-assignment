package auth

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"yozigo/internal/apperr"
)

const (
	minPasswordLen = 8
	// bcrypt only uses the first 72 bytes of input; longer passwords are rejected rather than
	// silently truncated.
	maxPasswordBytes = 72
)

// dummyHash is compared against when a login email does not exist, so that "unknown email"
// and "wrong password" take roughly the same time and cannot be told apart by timing.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("timing-equaliser-password"), bcrypt.DefaultCost)

func ValidatePassword(pw string) error {
	if utf8.RuneCountInString(pw) < minPasswordLen {
		return apperr.New(apperr.Invalid, "WEAK_PASSWORD", "Password must be at least 8 characters.")
	}
	if len(pw) > maxPasswordBytes {
		return apperr.New(apperr.Invalid, "PASSWORD_TOO_LONG", "Password must be at most 72 bytes.")
	}
	return nil
}

func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// NormalizeEmail trims and lower-cases an email and checks it is a plain address.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 || !strings.Contains(email[strings.LastIndex(email, "@")+1:], ".") {
		return "", apperr.New(apperr.Invalid, "INVALID_EMAIL", "Please enter a valid email address.")
	}
	return email, nil
}
