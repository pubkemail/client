// +build dev

package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/njones/logger"
	flag "github.com/spf13/pflag"
)

func flags() []commonOptFunc {
	var version = flag.BoolP("version", "v", false, "the version")
	var webPortP = flag.IntP("web-port", "p", 18810, "the port to use for the webserver")
	var logPortP = flag.IntP("log-port", "", 0, "the port to use for sending logs to")
	var webPrefixP = flag.StringP("web-prefix", "", "dev", "the prefix to use for the webserver")
	var afterDateP = flag.StringP("after", "a", "", "the <Year>-<Month>-<Day>T<Hour>:<Minute>:<Second> <Timezone> to forward emails after. The T and time after is optional, use a timezone such as UTC,PDT for example.")

	flag.Parse()

	log = log.Suppress(logger.LevelTrace)

	if *logPortP != 0 {
		conn, err := net.Dial("tcp", fmt.Sprintf(":%d", *logPortP))
		if err != nil {
			panic(err)
		}
		log = logger.New(logger.WithOutput(conn))
	}

	if *version {
		fmt.Println("-- Development version --")
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
		func(c *common) { c.web.randPrefix = *webPrefixP },
		func(c *common) { c.term.check.afterDate = afterDate },
		func(c *common) { c.web.useLocalFS = true },
	}
}
