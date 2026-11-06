package db

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func hostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestKnownHostsSkipsUnknownKeyTypes(t *testing.T) {
	known, other := hostKey(t), hostKey(t)
	path := filepath.Join(t.TempDir(), "known_hosts")
	text := "# comment\n\n" +
		"newer.example.com ssh-mldsa44-ed25519 AAAAE3NzaC1tbGRzYTQ0LWVkMjU1MTk=\n" +
		knownhosts.Line([]string{"db.example.com"}, known) + "\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := knownhosts.New(path); err == nil {
		t.Fatal("expected x/crypto to reject the file, so this test covers the skip")
	}
	kh, err := loadKnownHosts(path)
	if err != nil {
		t.Fatal(err)
	}
	addr := &net.TCPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 22}
	if err := kh("db.example.com:22", addr, known); err != nil {
		t.Fatalf("known host: %v", err)
	}
	var ke *knownhosts.KeyError
	if err := kh("db.example.com:22", addr, other); !errors.As(err, &ke) || len(ke.Want) == 0 {
		t.Fatalf("changed key: want a mismatch, got %v", err)
	}
	if err := kh("newer.example.com:22", addr, known); !errors.As(err, &ke) || len(ke.Want) != 0 {
		t.Fatalf("skipped host: want unknown, got %v", err)
	}
}

func TestKnownHostsMissingFile(t *testing.T) {
	if _, err := loadKnownHosts(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error")
	}
}
