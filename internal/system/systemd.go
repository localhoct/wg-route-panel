package system

import (
	"context"
	"fmt"
)

type Systemd struct{ Runner Runner }

func (s Systemd) Action(ctx context.Context, action, unit string) error {
	switch action {
	case "start", "stop", "restart", "reload":
	default:
		return fmt.Errorf("invalid action")
	}
	_, e := s.Runner.Run(ctx, "sudo", "/usr/bin/systemctl", action, unit)
	return e
}
func (s Systemd) Active(ctx context.Context, unit string) bool {
	o, e := s.Runner.Run(ctx, "sudo", "/usr/bin/systemctl", "is-active", unit)
	return e == nil && o == "active\n"
}
