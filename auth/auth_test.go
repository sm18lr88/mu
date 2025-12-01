package auth

import (
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	tmpDir, _ := os.MkdirTemp("", "mu_test_auth")
	originalHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)

	code := m.Run()

	_ = os.Setenv("HOME", originalHome)
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestAccountLifecycle(t *testing.T) {
	acc := &Account{
		ID:      "testuser",
		Name:    "Test User",
		Secret:  "password123",
		Created: time.Now(),
	}

	if err := Create(acc); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	if err := Create(acc); err == nil {
		t.Error("Expected error creating duplicate account, got nil")
	}

	sess, err := Login("testuser", "password123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if sess == nil || sess.Token == "" {
		t.Error("Session token missing")
	}

	if _, err := Login("testuser", "wrongpassword"); err == nil {
		t.Error("Expected login failure for wrong password")
	}

	fetched, err := GetAccount("testuser")
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if fetched.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got %s", fetched.Name)
	}

	fetched.Member = true
	if err := UpdateAccount(fetched); err != nil {
		t.Errorf("UpdateAccount failed: %v", err)
	}

	updated, _ := GetAccount("testuser")
	if !updated.Member {
		t.Error("Account update not persisted")
	}

	if err := ValidateToken(sess.Token); err != nil {
		t.Errorf("Token validation failed: %v", err)
	}

	if err := Logout(sess.Token); err != nil {
		t.Errorf("Logout failed: %v", err)
	}

	if err := ValidateToken(sess.Token); err == nil {
		t.Error("Token should be invalid after logout")
	}

	if err := DeleteAccount("testuser"); err != nil {
		t.Errorf("DeleteAccount failed: %v", err)
	}

	if _, err := GetAccount("testuser"); err == nil {
		t.Error("Account should not exist after deletion")
	}
}
