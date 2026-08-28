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
		t.Fatal("insecure public HTTP accepted without explicit opt-in")
	}
	c.AllowInsecureHTTP = true
	if e := c.Validate(); e != nil {
		t.Fatalf("explicit insecure HTTP opt-in rejected: %v", e)
	}
	c.SecureCookies = true
	if e := c.Validate(); e == nil {
		t.Fatal("secure cookies accepted with HTTP base URL")
	}
}
func TestDefaultRuntimeUsesSupervisorAndPort9090(t *testing.T) {
	c := Default()
	if c.ListenAddr != "127.0.0.1:9090" || c.SingBox.Manager != "supervisor" {
		t.Fatalf("unexpected defaults: %#v", c)
	}
}

func TestDockerConfigAllowsExplicitHTTP(t *testing.T) {
	t.Setenv("PANEL_SESSION_SECRET", "12345678901234567890123456789012")
	c, err := Load("../../configs/panel.docker.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != "0.0.0.0:9090" || c.BaseURL != "http://localhost:9090" || !c.AllowInsecureHTTP || c.SecureCookies {
		t.Fatalf("unexpected Docker HTTP configuration: %#v", c)
	}
}

func TestTrustedProxiesValidation(t *testing.T) {
	c := Default()
	c.SessionSecret = "12345678901234567890123456789012"
	c.TrustedProxies = []string{"10.0.0.1", "192.168.1.0/24"}
	if e := c.Validate(); e != nil {
		t.Fatalf("valid trusted_proxies rejected: %v", e)
	}
	c.TrustedProxies = []string{"not-an-ip"}
	if e := c.Validate(); e == nil {
		t.Fatal("invalid trusted_proxies entry accepted")
	}
}
