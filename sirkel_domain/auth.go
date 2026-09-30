package sirkel_domain

const RoleSuperAdmin = "super_admin"

type Organization struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type User struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Email          string `json:"email"`
	Role           string `json:"role"`
}

// This one is used on the admin part
type UserComplete struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	Email          string  `json:"email"`
	Role           string  `json:"role"`
	Name           string  `json:"name"`
	FirstSurname   string  `json:"first_surname"`
	SecondSurname  string  `json:"second_surname"`
	Phone          *string `json:"phone"`
	// PasswordHash   string `json:"password_hash"`
}

// UserOrganization is one row of a user's membership in an organization
// (sirkel_engine.user_organizations). Role lives here, not on User, because a
// user's role can differ between the organizations they belong to. IsCurrent
// marks the single membership a user is currently operating under; it is
// what a login token is issued for.
type UserOrganization struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`
	IsCurrent      bool   `json:"is_current"`
}
