package sirkel_auth

import (
	"context"
	"os"
	"sirkel-engine-lib/sirkel_errors"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type LoginParams struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	Role  string `json:"role"`
	Email string `json:"email"`
}

func (h *SirkelAuthHandler) Login(params LoginParams) (LoginResponse, error) {
	// 2. Traer el usuario de la DB
	var userID uuid.UUID
	var passwordHash string
	var role string
	var emailVerifiedAt *time.Time

	query := `SELECT id, password_hash, role, email_verified_at, organization_id FROM sirkel_engine.users WHERE email=$1`

	var organizationID uuid.UUID
	err := h.Pool.QueryRow(context.Background(), query, params.Email).Scan(&userID, &passwordHash, &role, &emailVerifiedAt, &organizationID)
	if err != nil {
		// Usamos un mensaje genérico para no revelar si el email existe o no
		return LoginResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeInvalidCredentials, "credenciales inválidas", err)
	}

	// TODO: enable checking is email is verified
	// if emailVerifiedAt == nil {
	// 	return c.Status(400).JSON(fiber.Map{"error": "Usuario no verificado"})
	// }

	// Compare password
	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(params.Password))
	if err != nil {
		return LoginResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeInvalidCredentials, "credenciales inválidas", err)
	}

	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":              userID.String(),
		"role":            role,
		"exp":             time.Now().Add(time.Hour * 72).Unix(),
		"organization_id": organizationID,
		"platform":        "sirkel",
	})
	secret := os.Getenv("JWT_SECRET")
	t, err := token.SignedString([]byte(secret))
	if err != nil {
		return LoginResponse{}, sirkel_errors.Wrap(sirkel_errors.CodeTokenGenerationFailed, "error generando el token", err)
	}

	return LoginResponse{
		Token: t,
		Role:  role,
		Email: params.Email,
	}, nil
}
