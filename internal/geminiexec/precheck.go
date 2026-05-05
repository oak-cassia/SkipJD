package geminiexec

import (
	"errors"
	"os/exec"
)

// EnsureAvailable checks if the 'gemini' executable is available in the system's PATH.
// It returns an error if not found, to fail fast before starting a batch process.
func EnsureAvailable() error {
	_, err := exec.LookPath("gemini")
	if err != nil {
		return errors.New("gemini-cli not found on PATH — install it before running")
	}
	return nil
}
