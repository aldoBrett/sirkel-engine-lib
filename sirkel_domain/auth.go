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
