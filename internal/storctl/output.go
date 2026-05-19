package storctl

import (
	"fmt"
	"io"
	"strings"
)

type Reporter struct {
	out io.Writer
	err io.Writer
}

func NewReporter(out, err io.Writer) *Reporter {
	return &Reporter{out: out, err: err}
}

func (r *Reporter) OK(format string, args ...any) {
	fmt.Fprintf(r.out, "OK "+format+"\n", args...)
}

func (r *Reporter) Skip(format string, args ...any) {
	fmt.Fprintf(r.out, "SKIP "+format+"\n", args...)
}

func (r *Reporter) Warn(format string, args ...any) {
	fmt.Fprintf(r.out, "WARN "+format+"\n", args...)
}

func (r *Reporter) Fail(step, reason, next string) {
	fmt.Fprintf(r.err, "FAIL %s\n", step)
	if reason != "" {
		fmt.Fprintf(r.err, "reason: %s\n", strings.TrimSpace(reason))
	}
	if next != "" {
		fmt.Fprintf(r.err, "next: %s\n", next)
	}
}
