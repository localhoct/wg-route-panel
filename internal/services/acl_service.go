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

func (a *ACLService) Add(ctx context.Context, scope, value string) error {
	if !map[string]bool{"panel": true, "dns": true, "socks": true, "api": true}[scope] {
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
