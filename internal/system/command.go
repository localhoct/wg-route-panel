package system

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Runner interface {
	Run(context.Context, string, ...string) (string, error)
}
type ExecRunner struct{ Timeout time.Duration }

func (r ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	if r.Timeout == 0 {
		r.Timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out, er bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &er
	if e := cmd.Run(); e != nil {
		return out.String(), fmt.Errorf("%s failed: %w: %s", name, e, er.String())
	}
	return out.String(), nil
}
