package services

import (
	"context"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"github.com/localhoct/wg-route-panel/internal/system"
	"path/filepath"
	"testing"
)

type call struct {
	name string
	args []string
}
type fakeRunner struct{ calls *[]call }

func (f fakeRunner) Run(_ context.Context, n string, a ...string) (string, error) {
	*f.calls = append(*f.calls, call{n, a})
	return "", nil
}
func TestACLApplication(t *testing.T) {
	db, e := repository.InitDB(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	calls := []call{}
	s := ACLService{DB: db, NFT: system.NFTables{Runner: fakeRunner{&calls}}, Enabled: true}
	if e = s.Add(context.Background(), "dns", "192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	rules, e := repository.ACLRules(context.Background(), db)
	if e != nil || len(rules) != 1 || !rules[0].Applied {
		t.Fatalf("rules=%v err=%v", rules, e)
	}
	if len(calls) != 1 || calls[0].name != "sudo" || len(calls[0].args) != 4 || calls[0].args[2] != "dns_allow" {
		t.Fatalf("unexpected nft call %#v", calls)
	}
}
