package main

import (
	"github.com/njones/logger"
)

var log logger.Logger

// main kicks everything off... what can I say.
func main() {
	log = logger.New().Suppress(logger.LevelPrint)

	c := newCommon(flags()...)
	go c.webServer()
	c.terminal()
}
