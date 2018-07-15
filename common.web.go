package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi"
)

// friendlyError is the interface used to display
// messages and errors to the user facing web site
type friendlyError interface {
	Friendly() string
}

// webFriendlyErr is used to return errors that can displayed
// on the web page for users to see
type webFriendlyErr struct {
	error
	Text string
}

// Friendly satisfies the friendlyError interface
func (u webFriendlyErr) Friendly() string {
	return u.Text
}

// webFriendlyInfo is used to return information that can displayed
// on the web page for users to see
type webFriendlyInfo struct {
	Text string
}

// Error is to satisfy the error interface, even though it
// is not an error (but info can be passed back as an error)
func (u webFriendlyInfo) Error() string {
	return "no err (webFriendlyInfo): " + u.Text
}

// Friendly satisfies the friendlyError interface
func (u webFriendlyInfo) Friendly() string {
	return u.Text
}

// commonWeb holds all of the methods and web handlers that
// will be used by the user facing web page
type commonWeb struct {
	port       string
	portUpdate chan string
	templates  map[string]*template.Template

	randPrefix string
	useLocalFS bool
}

// funcsMap hold the functions that are accessiable via
// the template system
var funcsMap template.FuncMap = map[string]interface{}{
	"truncate": txtTruncate,
}

// loadTemplates loads all of the static template (which)
// usually define a template that can be used by the
// base templates. Concatenates them and prefixes them to
// the base template. Then parses the template, getting it
// ready to be filled in.
func (c *common) loadTemplates() error {
	fs := FS(c.web.useLocalFS)

	var rdrs []io.Reader
	for _, tmpl := range []string{
		"/assets/static/tmpls/footer.html",
		"/assets/static/tmpls/sidebar.html",
		"/assets/static/tmpls/style.html",
		"/assets/static/tmpls/style_pricing.html",
		"/assets/static/tmpls/top.html",
		"/assets/static/tmpls/bottom.html",
		"/assets/static/tmpls/loader.html",
	} {
		ft, err := fs.Open(tmpl)
		if err != nil {
			panic(fmt.Sprintf("%s - %v", tmpl, err))
		}
		defer ft.Close()
		rdrs = append(rdrs, ft)
	}

	for _, name := range []string{"/index.html", "/compose.html", "/email.html", "/pricing.html"} {
		f, err := fs.Open(name)
		if err != nil {
			panic(err)
		}

		mw := io.MultiReader(append(rdrs, f)...)

		b, err := ioutil.ReadAll(mw)
		if err != nil {
			panic(err)
		}
		f.Close()

		tmplName, err := template.New(name).Funcs(funcsMap).Parse(string(b))
		if err != nil {
			return webFriendlyErr{err, "the web templates could not be loaded"}
		}
		c.web.templates[name] = tmplName

		for _, r := range rdrs {
			r.(http.File).Seek(0, io.SeekStart)
		}
	}
	return nil
}

// webServer runs the webserver for the web page interface
func (c *common) webServer() {
	r := chi.NewRouter()
	r.Get(fmt.Sprintf("/%s/*", c.web.randPrefix), c.webGetHandler)
	r.Post(fmt.Sprintf("/%s*", c.web.randPrefix), c.webIndexHandler)

	listener, err := net.Listen("tcp", c.web.port)
	if err != nil {
		panic(err)
	}

	if c.web.port == ":0" {
		c.web.portUpdate <- fmt.Sprintf(":%d", listener.Addr().(*net.TCPAddr).Port)
	}
	close(c.web.portUpdate)
	http.Serve(listener, r) // no need for gracefull shutdown
}

// webGetHandler is the standard handler that displays each page
func (c *common) webGetHandler(w http.ResponseWriter, r *http.Request) {
	fn := "webGetHandler::"

	if c.web.useLocalFS {
		// always load the latest templates when using the local filesystem, for faster dev
		if err := c.loadTemplates(); err != nil {
			log.Warnf("%s load templates: %v", fn, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	name := chi.URLParam(r, "*")
	if name == "" {
		name = "/index.html"
	}

	if name[0] != '/' {
		name = "/" + name
	}

	var f io.ReadSeeker
	if page, ok := c.web.templates[name]; ok {
		c.Data.RequestURI, c.Data.RequestURIPath = r.RequestURI, path.Dir(r.RequestURI)

		switch name {
		case "/pricing.html":
			c.Data.TopFlags["pricing"] = "pricing"
		case "/compose.html", "/email.html":
			c.Data.BottomFlags["overlay"] = "overlay"
		}

		c.Data.TopFlags["page"] = name
		c.Data.TopFlags["title"] = fmt.Sprintf("%s - Pubkemail Web Interaface", strings.Title(strings.TrimSuffix(strings.TrimPrefix(name, "/"), ".html")))

		buf := new(bytes.Buffer)
		page.Execute(buf, c.Data)
		f = strings.NewReader(buf.String())
	} else {
		var err error
		fs := FS(c.web.useLocalFS)
		if f, err = fs.Open(name); err != nil {
			log.Warnf("%s fs open: %v", fn, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	http.ServeContent(w, r, name, time.Time{}, f)
}

// webIndexHandler handes the post back interaction from the index page
func (c *common) webIndexHandler(w http.ResponseWriter, r *http.Request) {
	fn := "webIndexHandler::"

	var err error
	var body []byte
	var values url.Values

	defer func() {
		c.Data.MainContentErrText = ""
		c.Data.MainContentInfoText = ""
		switch ferr := err.(type) {
		case webFriendlyErr:
			log.Warn(err)
			c.Data.MainContentErrText = ferr.Friendly()
		case webFriendlyInfo:
			c.Data.MainContentInfoText = ferr.Friendly()
		}
		http.Redirect(w, r, r.RequestURI, http.StatusSeeOther) // always redirect with a GET
	}()

	body, err = ioutil.ReadAll(r.Body)
	if err != nil {
		err = webFriendlyErr{
			fmt.Errorf("%s read body: %v", fn, err),
			"Something went wrong. Please Retry.",
		}
		return
	}

	values, err = url.ParseQuery(string(body))
	if err != nil {
		err = webFriendlyErr{
			fmt.Errorf("%s parse query: %v", fn, err),
			"Something went wrong. Please Retry.",
		}
		return
	}

	switch subVal := values.Get(c.Data.Const.Submit); subVal {
	case c.Data.Const.SubmitAddWIF:
		var wif WIF
		var fwdTo string

		wif, err = unmarshalWIF(values.Get(c.Data.Const.WIFStr))
		if err != nil {
			err = webFriendlyErr{
				fmt.Errorf("%s unmarshal wif: %v", fn, err),
				"Something went wrong. Please Retry.",
			}
			return
		}

		for fwdTo = range c.fwdDataMap {
			break // just grab a random one
		}

		c.Data.Addr.NewMail[wif.addr] = 0
		c.Data.Addr.Display[wif.addr] = AddrDisplay{
			Addr:  wif.addr,
			WIF:   wif.wif,
			FwdTo: fwdTo,
		}

		isFwd := len(c.Data.Addr.Display[wif.addr].FwdTo) > 0
		c.addrsDataMap[wif.addr] = addrData{
			isFwd:     isFwd,
			feedLinks: make(chan string, defaultFeedLinksChanLen),
			wif:       wif,
		}

		if isFwd {
			go c.termAddrChecker(wif.addr)
		}
		c.term.update.viewBottom <- addrDataMapToString(c.addrsDataMap)

		return
	case c.Data.Const.SubmitFwd, c.Data.Const.SubmitFwdTest:
		var fwdNameText = values.Get(c.Data.Const.FwdName)
		var fwdJSONText = values.Get(c.Data.Const.FwdJSON)
		var isTest = (subVal == c.Data.Const.SubmitFwdTest)

		if !isTest && len(strings.TrimSpace(fwdNameText)) == 0 {
			c.Data.FwdJSONText = fwdJSONText
			err = webFriendlyErr{
				fmt.Errorf("%s no fwd name text", fn),
				"The JSON submitted does not have a name. Please add a name and try again.",
			}
			return
		}

		if !isTest && len(strings.TrimSpace(fwdJSONText)) == 0 {
			err = webFriendlyInfo{
				fmt.Sprintf("You have deleted the Forwarding: %s.", fwdNameText),
			}

			for addr, display := range c.Data.Addr.Display {
				name := strings.Replace(fwdNameText, " ", "-", -1)
				if display.FwdTo == name {
					display.FwdTo = ""
					if data, ok := c.addrsDataMap[addr]; ok {
						data.isFwd = false
						c.addrsDataMap[addr] = data
					}
					c.Data.Addr.Display[addr] = display
					delete(c.fwdDataMap, name)
				}
			}
			return
		}

		var via fwdVia
		err = json.Unmarshal([]byte(fwdJSONText), &via)
		if err != nil {
			err = webFriendlyErr{
				fmt.Errorf("%s json unmarshal: %v", fn, err),
				"The JSON submitted is invalid. Please check and retry.",
			}
			return
		}

		var fwdEmail fwdEmailFunc
		switch {
		case via.fwdViaHTTPAPI != nil:
			// the wrapper to forward mails via HTTP API calls
			fwdEmail = func(from, subject, body string, headers mail.Header, isTest bool) error {
				return fwdHTTPAPIEmail(via, from, subject, body, headers, isTest)
			}
			err = webFriendlyInfo{
				fmt.Sprintf("You have added the HTTP API forwarding JSON: %s", fwdNameText),
			}
		case via.fwdViaSMTP != nil:
			// the wrapper to forward mails via SMTP calls
			fwdEmail = func(from, subject, body string, headers mail.Header, isTest bool) error {
				return fwdSMTPEmail(via, from, subject, body, headers, isTest)
			}
			err = webFriendlyInfo{
				fmt.Sprintf("You have added the SMTP forwarding JSON: %s", fwdNameText),
			}
		default:
			err = webFriendlyErr{
				fmt.Errorf("%s json unmarshal: %v", fn, err),
				"The JSON submitted is invalid. Please check and retry.",
			}
		}

		if isTest {
			c.Data.FwdNameText = fwdNameText
			c.Data.FwdJSONText = fwdJSONText

			testSubject := fmt.Sprintf("Testing 123 - %d", time.Now().Unix())
			testBody := "This is a test email sent @: " + time.Now().Format(time.RFC822)
			err := fwdEmail("test@example.com", testSubject, testBody, nil, isTest)
			log.OnErr(err).Printf("fwd email: %v", err)
			return
		}

		c.Data.FwdNameText = ""
		c.Data.FwdJSONText = ""

		name := strings.Replace(fwdNameText, " ", "-", -1)
		c.fwdDataMap[name] = fwdData{fwdEmail: fwdEmail}
		c.Data.Fwd.Display[name] = FwdDisplay{
			Name: fwdNameText,
			JSON: fwdJSONText,
		}

		return
	case c.Data.Const.SubmitFwdTo:
		for k, v := range values {
			if k == c.Data.Const.Submit {
				continue
			}

			if val, ok := c.Data.Addr.Display[k]; ok && len(v) > 0 {
				val.FwdTo = v[0]
				c.Data.Addr.Display[k] = val
			}
		}
		return
	}

	err = webFriendlyErr{
		fmt.Errorf("%s no submit type found for %s", fn, values.Get(c.Data.Const.Submit)),
		"Something went wrong. Please retry.",
	}

	return
}
