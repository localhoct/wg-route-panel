package services

import (
	"context"
	"database/sql"
	"errors"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"github.com/localhoct/wg-route-panel/internal/system"
)

type ACLService struct {
	DB      *sql.DB
	NFT     system.NFTables
	Enabled bool
}

// Valid ACL scopes intentionally do not include "dns": DNS is public by
// design (see deploy/nftables/wg-route-panel.nft and the panel's DNS
// settings page), so there is no allow-list to manage for it. panel/api
// gate access to the web UI/JSON API; socks gates the SOCKS5 listener when
// it is bound beyond loopback.
func (a *ACLService) Add(ctx context.Context, scope, value string) error {
	if !map[string]bool{"panel": true, "api": true, "socks": true}[scope] {
		return errors.New("invalid scope")
	}
	cidr, v, e := system.NormalizeCIDR(value)
	if e != nil {
		return e
	}
	if e = repository.AddACL(ctx, a.DB, scope, cidr, v); e != nil {
		return e
	}
	if a.Enabled {
		set := scope + "_allow"
		if v == 6 {
			set += "6"
		}
		if e = a.NFT.Element(ctx, true, set, cidr); e != nil {
			return e
		}
		_, _ = a.DB.ExecContext(ctx, "UPDATE acl_rules SET applied=1 WHERE scope=? AND cidr=?", scope, cidr)
	}
	return nil
}
func (a *ACLService) Delete(ctx context.Context, id int64) error {
	x, e := repository.ACLByID(ctx, a.DB, id)
	if e != nil {
		return e
	}
	if a.Enabled && x.Applied {
		set := x.Scope + "_allow"
		if x.IPVersion == 6 {
			set += "6"
		}
		if e = a.NFT.Element(ctx, false, set, x.CIDR); e != nil {
			return e
		}
	}
	return repository.DeleteACL(ctx, a.DB, id)
}
