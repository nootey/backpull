//go:build windows

package sshclient

import (
	"net"

	"github.com/Microsoft/go-winio"
)

// Windows OpenSSH agent listens on a named pipe, not SSH_AUTH_SOCK.
const agentPipe = `\\.\pipe\openssh-ssh-agent`

func dialAgent() (net.Conn, error) {
	return winio.DialPipe(agentPipe, nil)
}
