package system

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

var ifaceRE = regexp.MustCompile(`^[A-Za-z0-9_=+.-]{1,15}$`)

func ValidateInterface(s string) error {
	if !ifaceRE.MatchString(s) {
		return errors.New("invalid interface name")
	}
	return nil
}
func NormalizeCIDR(v string) (string, int, error) {
	v = strings.TrimSpace(v)
	if ip := net.ParseIP(v); ip != nil {
		if ip.To4() != nil {
			return ip.String() + "/32", 4, nil
		}
		return ip.String() + "/128", 6, nil
	}
	ip, n, e := net.ParseCIDR(v)
	if e != nil {
		return "", 0, errors.New("invalid IP or CIDR")
	}
	if ip.To4() != nil {
		return n.String(), 4, nil
	}
	return n.String(), 6, nil
}
func ValidateDomain(v string) error {
	v = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
	if len(v) < 1 || len(v) > 253 || strings.ContainsAny(v, " /:@") {
		return errors.New("invalid domain")
	}
	for _, p := range strings.Split(v, ".") {
		if len(p) < 1 || len(p) > 63 {
			return errors.New("invalid domain label")
		}
	}
	return nil
}
func ValidateWGConfig(raw string) error {
	s := bufio.NewScanner(strings.NewReader(raw))
	section := ""
	iface := map[string]bool{}
	peers := []map[string]bool{}
	for s.Scan() {
		line := strings.TrimSpace(strings.SplitN(s.Text(), "#", 2)[0])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[] ")
			if section == "Peer" {
				peers = append(peers, map[string]bool{})
			}
			continue
		}
		p := strings.SplitN(line, "=", 2)
		if len(p) != 2 {
			return fmt.Errorf("invalid line: %q", line)
		}
		k := strings.TrimSpace(p[0])
		v := strings.TrimSpace(p[1])
		if v == "" {
			return fmt.Errorf("empty %s", k)
		}
		if section == "Interface" {
			iface[k] = true
		} else if section == "Peer" && len(peers) > 0 {
			peers[len(peers)-1][k] = true
		}
	}
	if e := s.Err(); e != nil {
		return e
	}
	if !iface["PrivateKey"] || !iface["Address"] {
		return errors.New("[Interface] requires PrivateKey and Address")
	}
	if len(peers) == 0 {
		return errors.New("at least one [Peer] is required")
	}
	for i, p := range peers {
		if !p["PublicKey"] || !p["AllowedIPs"] {
			return fmt.Errorf("peer %d requires PublicKey and AllowedIPs", i+1)
		}
	}
	return nil
}
