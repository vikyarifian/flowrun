package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ExecuteCommand runs arbitrary shell commands, capturing stdout and stderr
// and printing them in real-time to the terminal while returning the raw combined output.
func ExecuteCommand(commandStr string) (int, string) {
	var buf bytes.Buffer
	mwStdout := io.MultiWriter(os.Stdout, &buf)
	mwStderr := io.MultiWriter(os.Stderr, &buf)

	cmd := exec.Command("sh", "-c", commandStr)
	cmd.Stdout = mwStdout
	cmd.Stderr = mwStderr

	err := cmd.Run()
	statusCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			statusCode = exitError.ExitCode()
		} else {
			statusCode = -1
			fmt.Fprintf(mwStderr, "\n[flowrun process error] Execution failed: %v\n", err)
		}
	}
	return statusCode, buf.String()
}
