package system

import "testing"

func TestValidateWGConfig(t *testing.T) {
	good := "[Interface]\nPrivateKey = secret\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = peer\nAllowedIPs = 0.0.0.0/0\n"
	if err := ValidateWGConfig(good); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	for _, bad := range []string{"[Peer]\nPublicKey=x\nAllowedIPs=0.0.0.0/0", "[Interface]\nPrivateKey=x\nAddress=1.2.3.4"} {
		if ValidateWGConfig(bad) == nil {
			t.Errorf("accepted invalid config %q", bad)
		}
	}
}
func TestNormalizeCIDR(t *testing.T) {
	cases := []struct {
		in, want string
		version  int
	}{{"192.0.2.4", "192.0.2.4/32", 4}, {"2001:db8::/64", "2001:db8::/64", 6}}
	for _, tc := range cases {
		got, v, e := NormalizeCIDR(tc.in)
		if e != nil || got != tc.want || v != tc.version {
			t.Errorf("%s: got %s/%d, %v", tc.in, got, v, e)
		}
	}
	if _, _, e := NormalizeCIDR("not-an-ip"); e == nil {
		t.Error("invalid CIDR accepted")
	}
}
func TestValidateDomain(t *testing.T) {
	if ValidateDomain("example.com") != nil {
		t.Fatal("valid rejected")
	}
	if ValidateDomain("bad domain") == nil {
		t.Fatal("invalid accepted")
	}
}
