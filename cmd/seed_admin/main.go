// Command seed_admin interactively creates an admin user in
// sirkel_engine.users. It is meant to be run once, by hand, to bootstrap the
// first admin account for an environment. Unlike cmd/seed, it is not
// idempotent reference-data loading: it prompts for the account's details
// instead of taking them as flags or env vars, so a password never ends up
// in shell history or a process list, and it fails if the email is already
// taken rather than silently upserting over an existing account.
//
// Usage:
//
//	DATABASE_URL=postgres://user:pass@host:5432/dbname go run ./cmd/seed_admin
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("seed_admin: %v", err)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	details, err := promptForAdminDetails()
	if err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(details.password), 10)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	id := uuid.New()
	query := `INSERT INTO sirkel_engine.users (id, role, name, first_surname, second_surname, email, password_hash)
		VALUES ($1, 'super_admin', $2, $3, $4, $5, $6)`
	_, err = pool.Exec(ctx, query, id, details.name, details.firstSurname, details.secondSurname, details.email, string(hash))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("a user with email %q already exists", details.email)
		}
		return fmt.Errorf("insert admin user: %w", err)
	}

	log.Printf("seed_admin: created admin user %s (%s)", details.email, id)
	return nil
}

type adminDetails struct {
	email         string
	password      string
	name          string
	firstSurname  string
	secondSurname string
}

func promptForAdminDetails() (*adminDetails, error) {
	reader := bufio.NewReader(os.Stdin)

	email, err := promptLine(reader, "Admin email: ")
	if err != nil {
		return nil, err
	}
	if email == "" {
		return nil, fmt.Errorf("email must not be empty")
	}

	name, err := promptLine(reader, "First name: ")
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("first name must not be empty")
	}

	firstSurname, err := promptLine(reader, "First surname: ")
	if err != nil {
		return nil, err
	}
	if firstSurname == "" {
		return nil, fmt.Errorf("first surname must not be empty")
	}

	secondSurname, err := promptLine(reader, "Second surname: ")
	if err != nil {
		return nil, err
	}
	if secondSurname == "" {
		return nil, fmt.Errorf("second surname must not be empty")
	}

	password, err := promptPassword("Admin password: ")
	if err != nil {
		return nil, err
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	confirmation, err := promptPassword("Confirm password: ")
	if err != nil {
		return nil, err
	}
	if password != confirmation {
		return nil, fmt.Errorf("passwords do not match")
	}

	return &adminDetails{
		email:         email,
		password:      password,
		name:          name,
		firstSurname:  firstSurname,
		secondSurname: secondSurname,
	}, nil
}

func promptLine(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(password), nil
}

func connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}
