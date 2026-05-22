package storctl

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Runner interface {
	Run(name string, args ...string) (string, error)
	Sh(command string) (string, error)
	Exists(name string) bool
}

type OSRunner struct {
	env []string
}

type CommandError struct {
	Command string
	Output  string
	Err     error
}

func (e CommandError) Error() string {
	output := strings.TrimSpace(e.Output)
	if output == "" {
		return fmt.Sprintf("%s: %v", e.Command, e.Err)
	}
	return fmt.Sprintf("%s: %v: %s", e.Command, e.Err, output)
}

func NewOSRunner(proxy, noProxy string) *OSRunner {
	env := os.Environ()
	if proxy != "" {
		env = append(env, "http_proxy="+proxy, "https_proxy="+proxy, "HTTP_PROXY="+proxy, "HTTPS_PROXY="+proxy)
	}
	if noProxy != "" {
		env = append(env, "no_proxy="+noProxy, "NO_PROXY="+noProxy)
	}
	return &OSRunner{env: env}
}

func (r *OSRunner) Run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return out.String(), CommandError{Command: name + " " + strings.Join(args, " "), Output: out.String(), Err: ctx.Err()}
	}
	if err != nil {
		return out.String(), CommandError{Command: name + " " + strings.Join(args, " "), Output: out.String(), Err: err}
	}
	return out.String(), nil
}

func (r *OSRunner) Sh(command string) (string, error) {
	if simMode() {
		return r.Run("storctl-sim-sh", command)
	}
	return r.Run("/bin/sh", "-c", command)
}

func (r *OSRunner) Exists(name string) bool {
	if simMode() {
		for _, missing := range strings.Split(os.Getenv("STORCTL_SIM_MISSING_COMMANDS"), ",") {
			if strings.TrimSpace(missing) == name {
				return false
			}
		}
	}
	_, err := exec.LookPath(name)
	return err == nil
}
