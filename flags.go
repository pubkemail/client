// +build !dev

package main

import (
	"fmt"
	"os"
	"time"

	flag "github.com/spf13/pflag"
)

// flags returns the flags options used during startup with the values
// returned as functional options
func flags() []commonOptFunc {
	// don't allow the prefix to be changed.
	var version = flag.BoolP("version", "v", false, "the version")
	var webPortP = flag.IntP("web-port", "p", 0, "the port to use for the webserver")
	var afterDateP = flag.StringP("after", "a", "", "the <Year>-<Month>-<Day>T<Hour>:<Minute>:<Second> <Timezone> to forward emails after. The T and time after is optional, use a timezone such as UTC,PDT for example.")

	flag.Parse()

	if *version {
		fmt.Printf("pubkemail CLI version: %s\npubkemaik API version: v1\ncopyright (c) pubkemail.com 2018\n", verSemVer)
		os.Exit(0)
	}

	afterDate, err := time.Parse("2006-01-02T15:04:05 MST", *afterDateP)
	if err != nil {
		if afterDate, err = time.Parse("2006-01-02", *afterDateP); err != nil {
			afterDate = time.Time{}
		}
	}

	return []commonOptFunc{
		func(c *common) { c.web.port = fmt.Sprintf(":%d", *webPortP) },
		func(c *common) { c.web.randPrefix = randPrefix(defaultRandPrefixByteLen) },
		func(c *common) { c.term.check.afterDate = afterDate },
	}
}
