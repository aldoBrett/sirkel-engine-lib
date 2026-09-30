package sirkel_auth

import (
	"os"
	"time"

	"sirkel-engine-lib/sirkel_errors"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
)

const tokenTTL = time.Hour * 72

// issueToken signs a JWT carrying a user's identity, role, and current organization. Login and RefreshToken
// both call this. Whenever a user's role or organization changes after their token was issued (see
// MoveUserOrganization), that token keeps the old claims until it expires or the holder calls RefreshToken.
func issueToken(userID uuid.UUID, role string, organizationID uuid.UUID) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":              userID.String(),
		"role":            role,
		"exp":             time.Now().Add(tokenTTL).Unix(),
		"organization_id": organizationID,
		"platform":        "sirkel",
	})
	secret := os.Getenv("JWT_SECRET")
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", sirkel_errors.Wrap(sirkel_errors.CodeTokenGenerationFailed, "error generando el token", err)
	}

	return signed, nil
}
