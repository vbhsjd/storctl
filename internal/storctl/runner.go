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
		return out.String(), fmt.Errorf("%s timed out", name)
	}
	if err != nil {
		return out.String(), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out.String(), nil
}

func (r *OSRunner) Sh(command string) (string, error) {
	return r.Run("/bin/sh", "-c", command)
}

func (r *OSRunner) Exists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
