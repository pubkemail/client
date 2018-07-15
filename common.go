package main

import (
	"fmt"
	"html/template"
	"sync"
	"time"
)

// defaultRandPrefixByteLen is the number of random bytes to use for the web interface prefix
const defaultRandPrefixByteLen = 6

// defaultWebPort is the port used for the website, 0, which is random
const defaultWebPort = 0

// defaultFeedLinksChanLen the number of feeds that can be queued up while checking
const defaultFeedLinksChanLen = 250

// defaultTermCheckInterval the time in seconds to check the RSS feed for new messages
const defaultTermCheckInterval = 5

// defaultShortWait is the time to wait during intervals in seconds
const defaultShortWait = 10

// defaultLongWait is the time to wait after errors or during feed resets
const defaultLongWait = 65

// WIF holds everything needed to work with WIFs
type WIF struct {
	wif       string
	addr      string
	addrComp  string
	currency  string
	pubKey    []byte
	priKey    []byte
	sharedKey []byte
}

// AddrDisplay holds data that can be displayed on the
// user facing web page about an address
type AddrDisplay struct {
	WIF    string // truncated
	Addr   string
	CurAbv string
	FwdTo  string
}

// FwdDisplay holds data that can be displayed on the user facing
// webpage about a forwarding HTTP-API or SMTP json
type FwdDisplay struct {
	Name string
	JSON string
}

// addrData holds data that can be used to work with addresses
type addrData struct {
	isFwd         bool
	feedLinksDone *sync.WaitGroup
	feedLinks     chan string
	wif           WIF
}

// fwdData holds data that can be used to work with sending emails
type fwdData struct {
	fwdEmail fwdEmailFunc
}

// addrMail holds the new mail w/ mutex count
// so that it can be atomiclly incremented
type addrMail struct {
	m       *sync.Mutex
	NewMail map[string]int
}

// incrNewMailCnt incements the counter for an address atomically
func (a *addrMail) incrNewMailCnt(addr string) {
	a.m.Lock()
	a.NewMail[addr]++
	a.m.Unlock()
}

// common holds the common data and display items
// between the web and terminal interfaces
type common struct {

	// Data is the object use in the html template
	// for the web display
	Data struct {
		HasAfterDate bool

		MainContentErrText         string
		MainContentInfoText        string
		FwdNameText, FwdJSONText   string
		RequestURI, RequestURIPath string

		TopFlags    map[string]string
		BottomFlags map[string]string

		Fwd struct {
			Display map[string]FwdDisplay
		}
		Addr struct {
			Display map[string]AddrDisplay
			*addrMail
		}
		Const struct {
			WIFStr  string
			FwdName string
			FwdJSON string

			Submit        string
			SubmitAddWIF  string
			SubmitFwd     string
			SubmitFwdTest string
			SubmitFwdTo   string
		}
	}

	fwdDataMap   map[string]fwdData
	addrsDataMap map[string]addrData

	web  commonWeb
	term commonTerm
}

// commonOptFunc is the type for optional functions these
// will be applied at the end of the the defaults when
// returning a new common object
type commonOptFunc func(*common)

// newCommon sets up and returns a new common object
func newCommon(opts ...commonOptFunc) *common {

	// update the nested struct variables
	c := &common{}
	c.term.update.viewInterval = time.Duration(45)
	c.term.update.viewTop = make(chan viewTopData, 1)
	c.term.update.viewBottom = make(chan string, 1)

	c.term.check.startTime = time.Now().Format(time.RFC3339)
	c.term.check.intervalNextDuration = 5 * time.Second
	c.term.check.intervalResetDuration = defaultLongWait * time.Second
	c.term.check.intervalWaitDuration = defaultShortWait * time.Second
	c.term.feed.readInterval = time.Duration(60)

	c.web.port = fmt.Sprintf(":%d", defaultWebPort)
	c.web.portUpdate = make(chan string)
	c.web.templates = make(map[string]*template.Template)

	c.Data.HasAfterDate = !c.term.check.afterDate.IsZero()
	c.Data.TopFlags = make(map[string]string)
	c.Data.BottomFlags = make(map[string]string)

	c.Data.Addr.Display = make(map[string]AddrDisplay)
	c.Data.Addr.addrMail = &addrMail{
		m:       new(sync.Mutex),
		NewMail: make(map[string]int),
	}
	c.Data.Fwd.Display = make(map[string]FwdDisplay)

	c.Data.Const.WIFStr = "wif-str"
	c.Data.Const.FwdName = "fwd-name"
	c.Data.Const.FwdJSON = "fwd-json"
	c.Data.Const.Submit = "sub"
	c.Data.Const.SubmitAddWIF = "sub-add-wif"
	c.Data.Const.SubmitFwd = "sub-fwd"
	c.Data.Const.SubmitFwdTest = "sub-fwd-test"
	c.Data.Const.SubmitFwdTo = "sub-fwd-to"

	c.addrsDataMap = make(map[string]addrData)
	c.fwdDataMap = make(map[string]fwdData)

	for _, opt := range opts {
		opt(c)
	}

	err := c.loadTemplates()
	if err != nil {
		panic(err)
	}

	return c
}
