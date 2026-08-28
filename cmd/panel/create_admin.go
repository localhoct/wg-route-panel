package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/localhoct/wg-route-panel/internal/auth"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"os"
	"strings"
)

// createAdmin is a CLI fallback, not the normal way to bootstrap the panel.
// Day-to-day, the first administrator is created from the browser at
// /setup (see internal/api/router.go's Setup Wizard), which only requires
// opening the panel - no SSH access needed. This command still exists for
// disaster recovery (e.g. every administrator account is locked out or the
// database was restored without one) and always creates an additional
// user even if one already exists; it does not replace or reset passwords.
func createAdmin(args []string) error {
	c, e := load()
	if e != nil {
		return e
	}
	db, e := repository.InitDB(c.DBPath)
	if e != nil {
		return e
	}
	defer db.Close()
	user := "admin"
	if len(args) > 0 && args[0] != "" {
		user = args[0]
	}
	pass := os.Getenv("PANEL_ADMIN_PASSWORD_INIT")
	if pass == "" {
		fmt.Print("Admin password (input visible): ")
		pass, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		pass = strings.TrimSpace(pass)
	}
	h, e := auth.HashPassword(pass)
	if e != nil {
		return e
	}
	if e = repository.CreateUser(context.Background(), db, user, h); e != nil {
		return e
	}
	fmt.Printf("Created administrator %q\n", user)
	return nil
}
