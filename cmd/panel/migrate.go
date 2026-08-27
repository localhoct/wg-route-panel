package main

import "github.com/localhoct/wg-route-panel/internal/repository"

func migrate() error {
	c, e := load()
	if e != nil {
		return e
	}
	db, e := repository.InitDB(c.DBPath)
	if e == nil {
		e = db.Close()
	}
	return e
}
