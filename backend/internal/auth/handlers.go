package auth

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"yozigo/internal/apperr"
	"yozigo/internal/config"
	"yozigo/internal/httpx"
)

type Handler struct {
	store *Store
	cfg   config.Config
}

func NewHandler(store *Store, cfg config.Config) *Handler {
	return &Handler{store: store, cfg: cfg}
}

type signupRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type demoLoginRequest struct {
	Account string `json:"account"`
}

// POST /api/auth/signup
func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	// Unknown fields (e.g. "role": "admin") are rejected by Decode.
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		httpx.Error(w, r, apperr.New(apperr.Invalid, "INVALID_NAME", "Name is required (max 100 characters)."))
		return
	}
	email, err := NormalizeEmail(req.Email)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if isReservedEmail(email) {
		httpx.Error(w, r, apperr.New(apperr.Invalid, "RESERVED_EMAIL", "This email domain is reserved."))
		return
	}
	if err := ValidatePassword(req.Password); err != nil {
		httpx.Error(w, r, err)
		return
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, err := h.store.CreateCustomer(r.Context(), name, email, hash)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	h.startSession(w, r, u, http.StatusCreated)
}

// POST /api/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	// One generic error for every failure, so the response never reveals whether the email
	// exists or the password was wrong.
	invalid := apperr.New(apperr.Unauthorized, "INVALID_CREDENTIALS", "Invalid email or password.")

	email := strings.ToLower(strings.TrimSpace(req.Email))
	u, hash, err := h.store.UserWithHashByEmail(r.Context(), email)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if u == nil {
		CheckPassword(string(dummyHash), req.Password) // equalise timing
		httpx.Error(w, r, invalid)
		return
	}
	if !CheckPassword(hash, req.Password) {
		httpx.Error(w, r, invalid)
		return
	}
	h.startSession(w, r, u, http.StatusOK)
}

// POST /api/auth/demo-login  {"account": "admin" | "customer1" | "customer2" | "customer3"}
//
// Logs into one of the seeded demo accounts without a password. Safety properties:
//   - only enabled when DEMO_LOGIN_ENABLED=true (never in production);
//   - the client sends only an allowlisted key, never a user id, email or role;
//   - the account must already exist (created by the seed script) and have the expected role;
//     nothing is ever created here;
//   - the result is an ordinary session, so all normal authorization checks still apply.
func (h *Handler) DemoLogin(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.DemoLoginEnabled {
		httpx.Error(w, r, apperr.New(apperr.NotFound, "DEMO_LOGIN_DISABLED", "Demo login is disabled on this server."))
		return
	}
	var req demoLoginRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	acct, ok := demoAccountByKey(req.Account)
	if !ok {
		httpx.Error(w, r, apperr.New(apperr.Invalid, "UNKNOWN_DEMO_ACCOUNT", "Unknown demo account."))
		return
	}
	u, err := h.store.UserByEmail(r.Context(), acct.Email)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if u == nil {
		httpx.Error(w, r, apperr.New(apperr.NotFound, "DEMO_ACCOUNT_MISSING",
			"Demo account not found. Run the seed script (see README) to create the demo accounts."))
		return
	}
	if u.Role != acct.Role {
		slog.Warn("demo account has unexpected role", "account", acct.Key, "role", u.Role)
		httpx.Error(w, r, apperr.New(apperr.Conflict, "DEMO_ACCOUNT_MISCONFIGURED",
			"Demo account exists but does not have the expected role."))
		return
	}
	slog.Info("demo login", "account", acct.Key, "user_id", u.ID)
	h.startSession(w, r, u, http.StatusOK)
}

// POST /api/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
		if err := h.store.DeleteSession(r.Context(), c.Value); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	http.SetCookie(w, h.cookie("", time.Unix(0, 0), -1))
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/auth/me
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"user": CurrentUser(r.Context())})
}

func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, u *User, status int) {
	// Replace any session the browser already had (prevents session fixation).
	if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
		_ = h.store.DeleteSession(r.Context(), c.Value)
	}
	token, expires, err := h.store.CreateSession(r.Context(), u.ID, h.cfg.SessionTTL)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	http.SetCookie(w, h.cookie(token, expires, int(time.Until(expires).Seconds())))
	httpx.JSON(w, status, map[string]any{"user": u})
}

func (h *Handler) cookie(value string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: true,                 // not readable from JavaScript (XSS can't steal it)
		Secure:   h.cfg.CookieSecure,   // HTTPS only in production
		SameSite: http.SameSiteLaxMode, // not sent on cross-site POSTs (CSRF defence, layer 1)
	}
}
