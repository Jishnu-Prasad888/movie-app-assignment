// Package auth implements signup, login, demo login, logout, server-side sessions and the
// authentication/authorization middleware.
package auth

import "time"

const (
	RoleCustomer = "customer"
	RoleAdmin    = "admin"
)

// User is the authenticated principal. It is always loaded from the database via the
// session cookie; the client never tells us who it is or what role it has.
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

func (u *User) IsAdmin() bool { return u != nil && u.Role == RoleAdmin }
