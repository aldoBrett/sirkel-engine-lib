package sirkel_auth

import (
	"testing"

	"sirkel-engine-lib/sirkel_errors"

	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
)

func mustHash(t *testing.T, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		t.Fatalf("unable to hash password: %v", err)
	}
	return string(hash)
}

func TestSirkelAuthHandler_Login_Success(t *testing.T) {
	pool := testPool(t)
	t.Setenv("JWT_SECRET", "test-secret")

	organizationID := insertTestOrganization(t, pool, "Acme")
	userID := insertTestUser(t, pool, organizationID, "login@example.com", mustHash(t, "s3cret!"), "admin")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	resp, err := h.Login(LoginParams{Email: "login@example.com", Password: "s3cret!"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if resp.Email != "login@example.com" {
		t.Fatalf("expected email %q, got %q", "login@example.com", resp.Email)
	}
	if resp.Role != "admin" {
		t.Fatalf("expected role %q, got %q", "admin", resp.Role)
	}
	if resp.Token == "" {
		t.Fatal("expected non-empty token")
	}

	token, err := jwt.Parse(resp.Token, func(*jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	if err != nil {
		t.Fatalf("unable to parse token: %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		t.Fatalf("expected valid claims, got %+v", token.Claims)
	}
	if claims["id"] != userID {
		t.Fatalf("expected id claim %q, got %v", userID, claims["id"])
	}
	if claims["role"] != "admin" {
		t.Fatalf("expected role claim %q, got %v", "admin", claims["role"])
	}
	if claims["platform"] != "tickets" {
		t.Fatalf("expected platform claim %q, got %v", "tickets", claims["platform"])
	}
}

func TestSirkelAuthHandler_Login_WrongPassword(t *testing.T) {
	pool := testPool(t)
	organizationID := insertTestOrganization(t, pool, "Acme")
	insertTestUser(t, pool, organizationID, "login@example.com", mustHash(t, "s3cret!"), "user")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	_, err := h.Login(LoginParams{Email: "login@example.com", Password: "wrong"})
	assertErrorCode(t, err, sirkel_errors.CodeInvalidCredentials)
}

func TestSirkelAuthHandler_Login_UnknownEmail(t *testing.T) {
	pool := testPool(t)

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	_, err := h.Login(LoginParams{Email: "nobody@example.com", Password: "whatever"})
	assertErrorCode(t, err, sirkel_errors.CodeInvalidCredentials)
}
