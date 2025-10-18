package main

import (
	"log"
)

func debug(s string, x ...interface{}) {
	log.Printf("Gotify2Telegram::"+s, x...)
}
