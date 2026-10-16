package auth

import "strings"

// DemoAccount describes one of the seeded accounts reachable through demo login.
type DemoAccount struct {
	Key   string
	Name  string
	Email string
	Role  string
}

// DemoAccounts is the explicit allowlist for POST /api/auth/demo-login. The client sends only
// one of these keys; the server maps it to an email and the expected role. Anything else is
// rejected. The seed script creates exactly these accounts.
//
// The emails use the reserved ".test" TLD and the whole @yozigo.test domain is blocked for
// normal signup, so nobody can pre-register one of these addresses.
var DemoAccounts = []DemoAccount{
	{Key: "admin", Name: "Asha Admin", Email: "admin@yozigo.test", Role: RoleAdmin},
	{Key: "customer1", Name: "Ravi Kumar", Email: "customer1@yozigo.test", Role: RoleCustomer},
	{Key: "customer2", Name: "Meera Iyer", Email: "customer2@yozigo.test", Role: RoleCustomer},
	{Key: "customer3", Name: "Arjun Singh", Email: "customer3@yozigo.test", Role: RoleCustomer},
}

const reservedDemoDomain = "@yozigo.test"

func demoAccountByKey(key string) (DemoAccount, bool) {
	for _, a := range DemoAccounts {
		if a.Key == key {
			return a, true
		}
	}
	return DemoAccount{}, false
}

func isReservedEmail(email string) bool {
	return strings.HasSuffix(email, reservedDemoDomain)
}
