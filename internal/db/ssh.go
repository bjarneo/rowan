package db

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"rowan/internal/config"
)

// dialSSH connects to the bastion host. It checks the host key against
// ~/.ssh/known_hosts and uses ssh-agent and the configured key file.
func dialSSH(ctx context.Context, s config.SSH) (*ssh.Client, error) {
	if s.Host == "" {
		return nil, errors.New("the SSH tunnel needs a host")
	}
	home, _ := os.UserHomeDir()
	kh, err := knownhosts.New(filepath.Join(home, ".ssh", "known_hosts"))
	if err != nil {
		return nil, fmt.Errorf("can't read ~/.ssh/known_hosts: %w", err)
	}
	var auths []ssh.AuthMethod
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	if s.Key != "" {
		b, err := os.ReadFile(config.Expand(s.Key))
		if err != nil {
			return nil, fmt.Errorf("can't read SSH key %s: %w", s.Key, err)
		}
		signer, err := ssh.ParsePrivateKey(b)
		var missing *ssh.PassphraseMissingError
		switch {
		case errors.As(err, &missing):
			if len(auths) == 0 {
				return nil, fmt.Errorf("the key %s has a passphrase. Add it to ssh-agent with ssh-add, then connect again", s.Key)
			}
		case err != nil:
			return nil, fmt.Errorf("can't parse SSH key %s: %w", s.Key, err)
		default:
			auths = append(auths, ssh.PublicKeys(signer))
		}
	}
	if len(auths) == 0 {
		return nil, errors.New("no SSH credentials. Start ssh-agent or set a key file")
	}
	user := s.User
	if user == "" {
		user = os.Getenv("USER")
	}
	port := s.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(port))
	cc := &ssh.ClientConfig{
		User: user,
		Auth: auths,
		HostKeyCallback: func(host string, remote net.Addr, key ssh.PublicKey) error {
			err := kh(host, remote, key)
			var ke *knownhosts.KeyError
			if errors.As(err, &ke) && len(ke.Want) == 0 {
				return fmt.Errorf("%s is not in ~/.ssh/known_hosts. Run `ssh %s` once to add it", s.Host, s.Host)
			}
			if errors.As(err, &ke) {
				return fmt.Errorf("the host key of %s does not match ~/.ssh/known_hosts", s.Host)
			}
			return err
		},
		Timeout: 10 * time.Second,
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("can't reach SSH host %s: %w", addr, err)
	}
	conn, chans, reqs, err := ssh.NewClientConn(raw, addr, cc)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("SSH to %s failed: %w", s.Host, err)
	}
	return ssh.NewClient(conn, chans, reqs), nil
}
