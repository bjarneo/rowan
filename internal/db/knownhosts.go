package db

import (
	"bytes"
	"io"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// loadKnownHosts skips the entries x/crypto can't parse, such as newer
// OpenSSH key types, so one of them doesn't break the check for every host.
func loadKnownHosts(path string) (ssh.HostKeyCallback, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var kept bytes.Buffer
	for _, line := range bytes.Split(b, []byte("\n")) {
		if _, _, _, _, _, err := ssh.ParseKnownHosts(line); err == nil || err == io.EOF {
			kept.Write(line)
			kept.WriteByte('\n')
		}
	}
	f, err := os.CreateTemp("", "rowan-known-hosts-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(kept.Bytes())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	return knownhosts.New(f.Name())
}
