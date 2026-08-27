package system

import (
	"context"
	"fmt"
)

type NFTables struct{ Runner Runner }

func (n NFTables) Element(ctx context.Context, add bool, set, cidr string) error {
	allowed := map[string]bool{"dns_allow": true, "dns_allow6": true, "panel_allow": true, "panel_allow6": true, "socks_allow": true, "socks_allow6": true, "api_allow": true, "api_allow6": true}
	if !allowed[set] {
		return fmt.Errorf("invalid nft set")
	}
	op := "add"
	if !add {
		op = "delete"
	}
	_, e := n.Runner.Run(ctx, "sudo", "/usr/local/sbin/wgpanel-nft-element", op, set, cidr)
	return e
}
