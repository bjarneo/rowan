package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const keyringService = "rowan"

var fileMu sync.Mutex

func credentialsPath() string { return filepath.Join(Dir(), "credentials.json") }

// SetPassword stores a password and returns the reference that goes into
// config.toml. It uses the system keychain. When the keychain is not
// available, or ROWAN_CREDENTIALS=file is set, it writes credentials.json
// with mode 0600 and returns fallback=true so the caller can tell the user.
func SetPassword(id, password string) (ref string, fallback bool, err error) {
	if password == "" {
		_ = DeletePassword(id)
		return "", false, nil
	}
	if os.Getenv("ROWAN_CREDENTIALS") != "file" {
		if err := keyring.Set(keyringService, id, password); err == nil {
			removeFileSecret(id)
			return "keychain:rowan/" + id, false, nil
		}
	}
	if err := setFileSecret(id, password); err != nil {
		return "", false, err
	}
	return "file:" + id, true, nil
}

// Password resolves a reference from config.toml.
func Password(ref string) (string, error) {
	switch {
	case ref == "":
		return "", nil
	case strings.HasPrefix(ref, "keychain:"):
		id := ref[strings.LastIndex(ref, "/")+1:]
		p, err := keyring.Get(keyringService, id)
		if err != nil {
			return "", errors.New("the system keychain has no password for this connection. Edit the connection and enter the password again")
		}
		return p, nil
	case strings.HasPrefix(ref, "file:"):
		return fileSecret(strings.TrimPrefix(ref, "file:"))
	case strings.HasPrefix(ref, "env:"):
		return os.Getenv(strings.TrimPrefix(ref, "env:")), nil
	}
	return "", errors.New("unknown password reference in config.toml. Use keychain:, file: or env:")
}

// DeletePassword removes a stored password from both stores.
func DeletePassword(id string) error {
	if os.Getenv("ROWAN_CREDENTIALS") != "file" {
		_ = keyring.Delete(keyringService, id)
	}
	removeFileSecret(id)
	return nil
}

func readFileSecrets() map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(credentialsPath())
	if err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func setFileSecret(id, pw string) error {
	fileMu.Lock()
	defer fileMu.Unlock()
	m := readFileSecrets()
	m[id] = pw
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	return WriteFileAtomic(credentialsPath(), b, 0o600)
}

func fileSecret(id string) (string, error) {
	fileMu.Lock()
	defer fileMu.Unlock()
	if p, ok := readFileSecrets()[id]; ok {
		return p, nil
	}
	return "", errors.New("credentials.json has no password for this connection")
}

func removeFileSecret(id string) {
	fileMu.Lock()
	defer fileMu.Unlock()
	m := readFileSecrets()
	if _, ok := m[id]; !ok {
		return
	}
	delete(m, id)
	b, _ := json.MarshalIndent(m, "", "  ")
	_ = WriteFileAtomic(credentialsPath(), b, 0o600)
}
