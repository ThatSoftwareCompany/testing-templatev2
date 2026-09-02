package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ThatSoftwareCompany/testing-templatev2/internal/modules/auth"
	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/config"
	"github.com/ThatSoftwareCompany/testing-templatev2/internal/platform/db"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) != 3 || os.Args[1] != "-command" || os.Args[2] != "create-admin" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/auth -command create-admin")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration")
		os.Exit(1)
	}
	if !cfg.Database.Enabled {
		fmt.Fprintln(os.Stderr, "DATABASE_ENABLED=true is required to create an admin")
		os.Exit(1)
	}

	pool, err := db.Open(context.Background(), cfg.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database initialization failed")
		os.Exit(1)
	}
	defer pool.Close()

	email, password, err := readCredentials()
	if err != nil {
		fmt.Fprintln(os.Stderr, "unable to read admin credentials")
		os.Exit(1)
	}
	service := auth.NewProvisioner(auth.NewPostgresRepository(pool))
	user, err := service.CreateAdmin(context.Background(), email, password)
	if err != nil {
		if errors.Is(err, auth.ErrUserExists) {
			fmt.Fprintln(os.Stderr, "an account with that email already exists")
		} else {
			fmt.Fprintln(os.Stderr, "admin creation failed")
		}
		os.Exit(1)
	}
	fmt.Printf("created admin user %s with role internal_admin\n", user.Email)
}

func readCredentials() (string, string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stdout, "Email: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return "", "", err
	}
	fmt.Fprint(os.Stdout, "Password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", "", err
	}
	fmt.Fprint(os.Stdout, "Confirm password: ")
	confirmation, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stdout)
	if err != nil || string(password) != string(confirmation) {
		return "", "", fmt.Errorf("password confirmation failed")
	}
	return strings.TrimSpace(email), string(password), nil
}
