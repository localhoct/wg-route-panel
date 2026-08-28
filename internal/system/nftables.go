package system

import (
	"context"
	"fmt"
)

type NFTables struct{ Runner Runner }

// Note: "dns" is intentionally not one of these nftables sets. DNS is
// public-by-default (see deploy/nftables/wg-route-panel.nft) since the
// whole point of this service is that clients can resolve names through it
// from the public internet without extra firewall setup; a "dns" ACL scope
// still exists at the application layer (see acl_service.go) purely for
// operators who want to document/track allow-listed sources, but it is not
// wired into an nftables set the way panel/api/socks are.
func (n NFTables) Element(ctx context.Context, add bool, set, cidr string) error {
	allowed := map[string]bool{"panel_allow": true, "panel_allow6": true, "socks_allow": true, "socks_allow6": true, "api_allow": true, "api_allow6": true}
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
