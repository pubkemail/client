package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/mmcdole/gofeed"
	"github.com/njones/bitcoin-crypto/bitelliptic"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/algorithm"
	"golang.org/x/crypto/openpgp/ecdh"
	"golang.org/x/crypto/openpgp/packet"
)

// viewWidth the width of the terminal view
const viewWidth = 80

// viewTopHeight the height of the top portion of the terminal view
const viewTopHeight = 9

// message holds the email headers and raw body string
type Message struct {
	Header mail.Header
	Body   string
}

// commonTerm holds all of the terminal realted common stuff
type commonTerm struct {
	check struct {
		afterDate time.Time
		startTime string
		lastTime  string

		intervalWaitDuration  time.Duration
		intervalNextDuration  time.Duration
		intervalResetDuration time.Duration
	}

	update struct {
		viewInterval time.Duration
		viewTop      chan viewTopData
		viewBottom   chan string
	}

	feed struct {
		readInterval time.Duration
	}

	done chan struct{}
}

// viewTopData is the struct that is returned through
// the checking channel, to be displayed on the top
// panel in the terminal
type viewTopData struct {
	lastCheckTime string
}

// viewWriter is a struct that will handle writing
// to a view, it makes adding a string or overwriting
// a line easy.
type viewWriter struct {
	v *gocui.View
}

// newViewWriter returns a new viewWriter
func newViewWriter(v *gocui.View) *viewWriter {
	vw := new(viewWriter)
	vw.v = v
	return vw
}

// WriteStringAt writes a string on the line ln within a view
func (vw *viewWriter) WriteStringAt(text string, ln int) error {
	defer vw.v.SetCursor(vw.v.Cursor())

	vw.v.SetCursor(viewWidth-len(text)-1, ln)
	x, y := vw.v.Cursor()
	for i, r := range text {
		vw.v.SetCursor(x+i-1, y)
		vw.v.EditWrite(r)
	}
	return nil
}

// terminal starts a terminal view
func (c *common) terminal() {
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	g.SetManagerFunc(c.layout)

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, c.quit); err != nil {
		log.Panicln(err)
	}

	go c.termReadFeed()

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}
}

// layout initally writes data to the terminal view, then
// holds two go routines that accept updates to the top
// and bottom views.
func (c *common) layout(g *gocui.Gui) (err error) {
	_, maxY := g.Size()

	var vt, vb *gocui.View

	// the top view setup
	if vt, err = g.SetView("viewTop", 0, 0, viewWidth, viewTopHeight); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		vt.Overwrite = true

		fmt.Fprintln(vt, fmt.Sprintf(" pubkemail CLI version: %s", verSemVer))
		fmt.Fprintln(vt, " pubkemaik API version: v1")
		fmt.Fprintln(vt, " copyright (c) pubkemail.com 2018")
		fmt.Fprintln(vt, " "+strings.Repeat("-", viewWidth-2))
		fmt.Fprintln(vt, " Status:")
		fmt.Fprintln(vt, " First Check:")
		fmt.Fprintln(vt, " Last Check:")
		fmt.Fprintln(vt, " Web Interface:")

		for portUpdate := range c.web.portUpdate {
			c.web.port = portUpdate
		}

		webAddress := fmt.Sprintf("http://localhost%s/%s/", c.web.port, c.web.randPrefix)
		t := newViewWriter(vt)
		t.WriteStringAt("online", 4)
		t.WriteStringAt(c.term.check.startTime, 5)
		t.WriteStringAt(c.term.check.lastTime, 6)
		t.WriteStringAt(webAddress, 7)
	}

	// the bottom view setup
	if vb, err = g.SetView("viewBottom", 0, viewTopHeight, viewWidth, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		vb.Overwrite = true
		vb.SetCursor(0, 0)
	}

	// keep the top view updated
	go func() {
		t := newViewWriter(vt)

		for update := range c.term.update.viewTop {
			g.Update(func(g *gocui.Gui) error {
				v, err := g.View("viewTop")
				log.OnErr(err).Panicf("top view update: %v", err)

				for i, v := range []string{"", "", update.lastCheckTime, ""} {
					if v != "" {
						t.WriteStringAt(v, i+4)
					}
				}

				// v.Clear()
				fmt.Fprint(v, update)
				return nil
			})
		}
	}()

	// keep the bottom view updated
	go func() {
		for update := range c.term.update.viewBottom {
			g.Update(func(g *gocui.Gui) error {
				v, err := g.View("viewBottom")
				log.OnErr(err).Panicf("bottom view update: %v", err)

				v.Clear()
				fmt.Fprint(v, update)
				return nil
			})
		}
	}()

	return nil
}

// quit is called when quiting the terminal application
func (c *common) quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

// checkMetaLink takes the data from a meta link and checks to see if it is hashed
// by the shared key of any private keys that were submitted.
func checkMetaLink(wif WIF, id, conHash, timestamp string) (hash string, ok bool) {
	mmac := hmac.New(sha256.New, wif.sharedKey)
	fmt.Fprintf(mmac, "com.pubkemail.meta.v1:%s/%s:%s", wif.addr, conHash, timestamp)
	if ok = id == hex.EncodeToString(mmac.Sum(nil)); ok {
		macc := hmac.New(sha256.New, wif.sharedKey)
		fmt.Fprintf(macc, "com.pubkemail.content.v1:%s/%s:%s", wif.addr, conHash, timestamp)
		hash = hex.EncodeToString(macc.Sum(nil))
	}
	return hash, ok
}

// getContentMsg grabs the content from the web and decodes the message using the
// shared key based on a supplied private key
func getContentMsg(wif WIF, contentHash string, ts time.Time) (*Message, error) {
	contentHashURL := fmt.Sprintf("https://content.pubkemail.com/v1/%s", contentHash)
	resp, err := http.Get(contentHashURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP from %s err: %v", contentHashURL, err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP from %s status: %v", contentHashURL, http.StatusText(resp.StatusCode))
	}

	encPriKey := &ecdh.PrivateKey{
		D: wif.priKey,
		PublicKey: ecdh.PublicKey{
			Curve: bitelliptic.S256(),
			KDF: ecdh.KDF{
				Hash:   algorithm.SHA512,
				Cipher: algorithm.AES256,
			},
		},
	}
	encPriKey.PublicKey.X, encPriKey.PublicKey.Y = bitelliptic.S256().ScalarBaseMult(wif.priKey)
	sigPriKey, err := ecdsa.GenerateKey(bitelliptic.S256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate signing private key err: %v", err)
	}

	data := encData{timestamp: ts, name: "Undisclosed", comment: "", email: "Undisclosed"}

	decEntity, err := newEntity(data, sigPriKey, encPriKey)
	if err != nil {
		return nil, fmt.Errorf("creating a new entity err: %v", err)
	}

	decBody := base64.NewDecoder(base64.StdEncoding, resp.Body)
	pgpMsg, err := openpgp.ReadMessage(decBody, openpgp.EntityList{decEntity}, nil, &packet.Config{
		DefaultHash: crypto.RIPEMD160,
	})
	if err != nil {
		return nil, fmt.Errorf("decrypt message err: %v", err)
	}

	msg, err := mail.ReadMessage(pgpMsg.UnverifiedBody)
	if err != nil {
		err = fmt.Errorf("parsing message: %v", err)
	}

	b, err := ioutil.ReadAll(msg.Body)
	if err != nil {
		err = fmt.Errorf("reading message: %v", err)
	}

	return &Message{Header: msg.Header, Body: string(b)}, err
}

// termAddrChecker is a function that holds the channel that links
// are sent back on to be checked against. If it's valid it will
// initiate a download and forward the message using the supplied
// forwarding data
func (c *common) termAddrChecker(addr string) { // checks if link is \
	for {
		select {
		case link := <-c.addrsDataMap[addr].feedLinks:

			u, err := url.Parse(link)
			if err != nil {
				log.Warnf("address check parse link url (%q): %v", link, err)
				continue
			}

			wif := c.addrsDataMap[addr].wif
			if contentEmailHash, ok := checkMetaLink(wif, u.Query().Get("check"), u.Query().Get("hash"), u.Query().Get("ts")); ok {
				c.Data.Addr.incrNewMailCnt(addr)

				ts, err := strconv.ParseInt(u.Query().Get("ts"), 10, 64)
				if err != nil {
					log.Warnf("parsing timestamp err: %v", err)
					continue
				}

				tsThen := time.Unix(0, ts)
				if !tsThen.After(c.term.check.afterDate) {
					continue
				}

				message, err := getContentMsg(wif, contentEmailHash, tsThen)
				if err != nil {
					log.Warnf("retriving email message: %v", err)
					continue
				}

				addrDisplay, ok := c.Data.Addr.Display[addr]
				if !ok {
					log.Warnf("display for address not found")
					continue
				}

				fwdTo := addrDisplay.FwdTo
				if fn, ok := c.fwdDataMap[fwdTo]; ok {
					from := message.Header.Get("From")
					subj := message.Header.Get("Subject")
					fn.fwdEmail(from, subj, message.Body, message.Header, false)
				}
			}

			c.term.update.viewTop <- viewTopData{lastCheckTime: time.Now().Format(time.RFC3339)}
		case <-c.term.done:
			c.addrsDataMap[addr].feedLinksDone.Done()
			break
		}
		time.Sleep(c.term.check.intervalWaitDuration)
	}
}

// termReadFeed checks the RSS feed on an interval and sends back the meta links
// it finds to be checked against the shared keys of the supplied addresses
func (c *common) termReadFeed() {
	page, limit := 1, 250
	for {
		log.Println("checking...")
		if len(c.addrsDataMap) == 0 {
			time.Sleep(c.term.check.intervalNextDuration)
			continue
		}
		fp := gofeed.NewParser()
		feedURL := fmt.Sprintf("https://rss.pubkemail.com/feed?page=%d&limit=%d", page, limit)
		feed, err := fp.ParseURL(feedURL)
		if err != nil {
			log.Warnf("gathering the feed URL", err)
			page = 1
			time.Sleep(c.term.check.intervalResetDuration)
			continue
		}
		if len(feed.Items) == 0 {
			page = 1
			time.Sleep(c.term.check.intervalNextDuration)
			break
		}
		for _, item := range feed.Items {
			if item.GUID == "GENESIS-ITEM" {
				page = 1
				time.Sleep(c.term.check.intervalResetDuration)
				break
			}
			for _, f := range c.addrsDataMap {
				f.feedLinks <- item.Link
			}
			page++
		}
		time.Sleep(c.term.check.intervalWaitDuration)
	}
}
