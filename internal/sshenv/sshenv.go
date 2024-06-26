// Package sshenv provides functionality for handling SSH environment variables
package sshenv

import (
	"os"
	"strings"
)

const (
	// GitProtocolEnv defines the ENV name holding the git protocol used
	GitProtocolEnv = "GIT_PROTOCOL"
	// SSHConnectionEnv defines the ENV holding the SSH connection
	SSHConnectionEnv = "SSH_CONNECTION"
	// SSHOriginalCommandEnv defines the ENV containing the original SSH command
	SSHOriginalCommandEnv = "SSH_ORIGINAL_COMMAND"
)

// Env represents the SSH environment variables
type Env struct {
	GitProtocolVersion string
	IsSSHConnection    bool
	OriginalCommand    string
	RemoteAddr         string
	NamespacePath      string
}

// NewFromEnv creates a new Env instance based on the current environment variables
func NewFromEnv() Env {
	isSSHConnection := false
	if ok := os.Getenv(SSHConnectionEnv); ok != "" {
		isSSHConnection = true
	}

	return Env{
		GitProtocolVersion: os.Getenv(GitProtocolEnv),
		IsSSHConnection:    isSSHConnection,
		RemoteAddr:         remoteAddrFromEnv(),
		OriginalCommand:    os.Getenv(SSHOriginalCommandEnv),
	}
}

// remoteAddrFromEnv returns the connection address from ENV string
func remoteAddrFromEnv() string {
	address := os.Getenv(SSHConnectionEnv)

	if address != "" {
		return strings.Fields(address)[0]
	}
	return ""
}
