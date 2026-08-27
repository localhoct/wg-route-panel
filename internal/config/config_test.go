package config

import "testing"

func TestValidationSecureDefaults(t *testing.T) {
	c := Default()
	c.SessionSecret = "12345678901234567890123456789012"
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.ListenAddr = "0.0.0.0:9090"
	if e := c.Validate(); e == nil {
		t.Fatal("insecure public HTTP accepted")
	}
}
func TestDefaultRuntimeUsesSupervisorAndPort9090(t *testing.T) {
	c := Default()
	if c.ListenAddr != "127.0.0.1:9090" || c.SingBox.Manager != "supervisor" || c.Xray.Manager != "supervisor" {
		t.Fatalf("unexpected defaults: %#v", c)
	}
}

func TestRemoteSocksRequiresAuthentication(t *testing.T) {
	c := Default()
	c.SessionSecret = "12345678901234567890123456789012"
	c.SOCKS.ListenAddr = "0.0.0.0"
	if e := c.Validate(); e == nil {
		t.Fatal("remote unauthenticated SOCKS accepted")
	}
	c.SOCKS.Username = "u"
	c.SOCKS.Password = "long-password"
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
}
