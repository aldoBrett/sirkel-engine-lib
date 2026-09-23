package sirkel_auth

import (
	"context"
	"testing"

	"sirkel-engine-lib/sirkel_errors"

	"golang.org/x/crypto/bcrypt"
)

func TestSirkelAuthHandler_CreateUser_Insert(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	organizationID := insertTestOrganization(t, pool, "Acme")

	phone := "555-1234"
	err := h.CreateUser(CreateUserParams{
		OrganizationID: organizationID,
		Email:          "new-user@example.com",
		Password:       "s3cret!",
		Name:           "Ada",
		FirstSurname:   "Lovelace",
		SecondSurname:  "Byron",
		Phone:          &phone,
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	var role, name, firstSurname, secondSurname, storedPhone, passwordHash string
	row := pool.QueryRow(context.Background(), `
		SELECT role, name, first_surname, second_surname, phone, password_hash
		FROM sirkel_engine.users WHERE email = $1
	`, "new-user@example.com")
	if err := row.Scan(&role, &name, &firstSurname, &secondSurname, &storedPhone, &passwordHash); err != nil {
		t.Fatalf("unable to read user: %v", err)
	}
	if role != "user" {
		t.Fatalf("expected role %q, got %q", "user", role)
	}
	if name != "Ada" || firstSurname != "Lovelace" || secondSurname != "Byron" {
		t.Fatalf("unexpected name fields: %q %q %q", name, firstSurname, secondSurname)
	}
	if storedPhone != phone {
		t.Fatalf("expected phone %q, got %q", phone, storedPhone)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("s3cret!")); err != nil {
		t.Fatalf("expected password hash to match, got error: %v", err)
	}
}

func TestSirkelAuthHandler_CreateUser_RequiredFields(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	organizationID := insertTestOrganization(t, pool, "Acme")

	base := CreateUserParams{
		OrganizationID: organizationID,
		Email:          "user@example.com",
		Password:       "s3cret!",
		Name:           "Ada",
		FirstSurname:   "Lovelace",
		SecondSurname:  "Byron",
	}

	tests := []struct {
		name    string
		mutate  func(p CreateUserParams) CreateUserParams
		wantErr sirkel_errors.Code
	}{
		{
			name:    "missing organization id",
			mutate:  func(p CreateUserParams) CreateUserParams { p.OrganizationID = ""; return p },
			wantErr: sirkel_errors.CodeOrganizationIDRequired,
		},
		{
			name:    "missing email",
			mutate:  func(p CreateUserParams) CreateUserParams { p.Email = ""; return p },
			wantErr: sirkel_errors.CodeEmailRequired,
		},
		{
			name:    "missing password",
			mutate:  func(p CreateUserParams) CreateUserParams { p.Password = ""; return p },
			wantErr: sirkel_errors.CodePasswordRequired,
		},
		{
			name:    "missing name",
			mutate:  func(p CreateUserParams) CreateUserParams { p.Name = ""; return p },
			wantErr: sirkel_errors.CodeNameRequired,
		},
		{
			name:    "missing first surname",
			mutate:  func(p CreateUserParams) CreateUserParams { p.FirstSurname = ""; return p },
			wantErr: sirkel_errors.CodeFirstSurnameRequired,
		},
		{
			name:    "missing second surname",
			mutate:  func(p CreateUserParams) CreateUserParams { p.SecondSurname = ""; return p },
			wantErr: sirkel_errors.CodeSecondSurnameRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.CreateUser(tt.mutate(base))
			assertErrorCode(t, err, tt.wantErr)
		})
	}
}

func TestSirkelAuthHandler_CreateUser_DuplicateEmail(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	organizationID := insertTestOrganization(t, pool, "Acme")

	params := CreateUserParams{
		OrganizationID: organizationID,
		Email:          "dup@example.com",
		Password:       "s3cret!",
		Name:           "Ada",
		FirstSurname:   "Lovelace",
		SecondSurname:  "Byron",
	}

	if err := h.CreateUser(params); err != nil {
		t.Fatalf("CreateUser() first insert error = %v", err)
	}

	err := h.CreateUser(params)
	assertErrorCode(t, err, sirkel_errors.CodeEmailAlreadyExists)
}
