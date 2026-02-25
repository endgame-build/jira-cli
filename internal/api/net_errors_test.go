package api

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	cliErrors "github.com/endgameio/jira-cli/internal/errors"
)

func TestMapNetworkError_ReturnsCLIError(t *testing.T) {
	err := mapNetworkError(fmt.Errorf("connection lost"), "test.atlassian.net")

	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.NETWORK_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.NETWORK_ERROR)
	}
	if cliErr.Context["instance"] != "test.atlassian.net" {
		t.Errorf("context.instance = %v, want %q", cliErr.Context["instance"], "test.atlassian.net")
	}
}

func TestMapNetworkError_WrapsOriginal(t *testing.T) {
	origErr := fmt.Errorf("original error")
	cliErr := mapNetworkError(origErr, "test.atlassian.net")

	if !errors.Is(cliErr, origErr) {
		t.Error("expected CLIError to wrap original error")
	}
}

func TestClassifyNetworkError_DNS(t *testing.T) {
	dnsErr := &net.DNSError{
		Err:  "no such host",
		Name: "bad.atlassian.net",
	}
	msg, suggestion := classifyNetworkError(dnsErr)
	if !strings.Contains(msg, "DNS") {
		t.Errorf("message = %q, want it to contain 'DNS'", msg)
	}
	if !strings.Contains(msg, "bad.atlassian.net") {
		t.Errorf("message = %q, want it to contain hostname", msg)
	}
	if suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestClassifyNetworkError_ConnectionRefused(t *testing.T) {
	sysErr := &os.SyscallError{
		Syscall: "connect",
		Err:     fmt.Errorf("connection refused"),
	}
	opErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: sysErr,
	}
	msg, suggestion := classifyNetworkError(opErr)
	if !strings.Contains(msg, "refused") {
		t.Errorf("message = %q, want it to contain 'refused'", msg)
	}
	if suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestClassifyNetworkError_Timeout(t *testing.T) {
	err := &mockTimeoutOpError{msg: "i/o timeout"}
	msg, suggestion := classifyNetworkError(err)
	if !strings.Contains(msg, "timed out") {
		t.Errorf("message = %q, want it to contain 'timed out'", msg)
	}
	if suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestClassifyNetworkError_Generic(t *testing.T) {
	err := fmt.Errorf("some unknown error")
	msg, suggestion := classifyNetworkError(err)
	if !strings.Contains(msg, "some unknown error") {
		t.Errorf("message = %q, want it to contain original error text", msg)
	}
	if suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestIsTimeout_True(t *testing.T) {
	err := &mockTimeoutOpError{msg: "timeout"}
	if !isTimeout(err) {
		t.Error("expected isTimeout to return true")
	}
}

func TestIsTimeout_False(t *testing.T) {
	err := fmt.Errorf("not a timeout")
	if isTimeout(err) {
		t.Error("expected isTimeout to return false for non-net.Error")
	}
}

// mockTimeoutOpError implements net.Error.
type mockTimeoutOpError struct {
	msg string
}

func (e *mockTimeoutOpError) Error() string   { return e.msg }
func (e *mockTimeoutOpError) Timeout() bool   { return true }
func (e *mockTimeoutOpError) Temporary() bool { return false }
