package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "create-admin":
			if e := createAdmin(os.Args[2:]); e != nil {
				log.Fatal(e)
			}
			return
		case "migrate":
			if e := migrate(); e != nil {
				log.Fatal(e)
			}
			return
		case "serve":
			if e := serve(); e != nil {
				log.Fatal(e)
			}
			return
		default:
			fmt.Fprintln(os.Stderr, "usage: panel [serve|create-admin|migrate]")
			os.Exit(2)
		}
	}
	if e := serve(); e != nil {
		log.Fatal(e)
	}
}
