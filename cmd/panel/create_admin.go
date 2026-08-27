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
