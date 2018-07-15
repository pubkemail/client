package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
)

// fwdEmailFunc is a function that sends email
type fwdEmailFunc func(from, subject, body string, headers mail.Header, isTest bool) error

// fwdVia holds all of the JSON types that can be
// decoded as pointers, only the ones filled in will
// be valid
type fwdVia struct {
	*fwdViaSMTP    `json:"smtp,omitempty"`
	*fwdViaHTTPAPI `json:"http-api,omitempty"`
}

// fwdViaSMTP is the JSON used for forwarding email via SMTP
type fwdViaSMTP struct {
	*fwdViaSMTPV1 `json:"v1,omitempty"`
}

// fwdViaSMTP is version 1
type fwdViaSMTPV1 struct {
	To      []string `json:"to"`
	User    *string  `json:"user,omitempty"`
	Pass    *string  `json:"pass,omitempty"`
	Address string   `json:"addr"`

	// only for testing
	From    string `json:"from,omitempty"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`
}

// fwdHTTP is the interface that needs to be satisfied by any http-api
// version. This is so we can nest different versions and override the
// response in the method of any updated structs
type fwdHTTP interface {
	To() []string
	User() *string
	Pass() *string
	URL() string
	Method() string
	Headers() map[string]string
	Parameters() map[string][]string
	IsMultipart() bool
}

// fwdViaHTTPAPI is the JSON used for forwarding email via an HTTP-API
type fwdViaHTTPAPI struct {
	*fwdViaHTTPAPIV1 `json:"v1,omitempty"`
}

// fwdViaHTTPAPIV1 is version 1
type fwdViaHTTPAPIV1 struct {
	ToVals         []string            `json:"to"`
	UserVal        *string             `json:"user,omitempty"`
	PassVal        *string             `json:"pass,omitempty"`
	URLVal         string              `json:"url"`
	MethodVal      string              `json:"method"`
	HeadersVals    map[string]string   `json:"headers,omitempty"`
	ParametersVals map[string][]string `json:"parameters,omitempty"`

	// only for testing... and should be passed through all future versions
	From    string `json:"from,omitempty"`
	Subject string `json:"subject,omitempty"`
	Text    string `json:"text,omitempty"`
	HTML    string `json:"html,omitempty"`
}

func (fwd *fwdViaHTTPAPIV1) To() []string                    { return fwd.ToVals }
func (fwd *fwdViaHTTPAPIV1) User() *string                   { return fwd.UserVal }
func (fwd *fwdViaHTTPAPIV1) Pass() *string                   { return fwd.PassVal }
func (fwd *fwdViaHTTPAPIV1) URL() string                     { return fwd.URLVal }
func (fwd *fwdViaHTTPAPIV1) Method() string                  { return fwd.MethodVal }
func (fwd *fwdViaHTTPAPIV1) Headers() map[string]string      { return fwd.HeadersVals }
func (fwd *fwdViaHTTPAPIV1) Parameters() map[string][]string { return fwd.ParametersVals }

// fwdSMTPEmail is the function that sends email via SMTP if a SMTP version has been defined
func fwdSMTPEmail(via fwdVia, from, subject, body string, headers mail.Header, isTest bool) (err error) {
	if isTest {
		if via.fwdViaSMTP.From != "" {
			from = via.fwdViaSMTP.From
		}
		if via.fwdViaSMTP.Subject != "" {
			subject = via.fwdViaSMTP.Subject
		}
		if via.fwdViaSMTP.Body != "" {
			body = via.fwdViaSMTP.Body
		}
	}

	var auth smtp.Auth
	if via.fwdViaSMTP.User != nil && via.fwdViaSMTP.Pass != nil {
		addr := strings.Split(via.fwdViaSMTP.Address, ":")[0]
		auth = smtp.PlainAuth("", *via.fwdViaSMTP.User, *via.fwdViaSMTP.Pass, addr)
	}

	// sort the order of the headers so that to, from and subject are last
	var keys []string
	for k, _ := range headers {
		l := strings.ToLower(k)
		if l == "to" || l == "from" || l == "subject" {
			continue
		}
		keys = append(keys, k)
	}
	keys = append(keys, []string{"To", "From", "Subject"}...)

	// Setup message
	var msg string
	for _, k := range keys {
		msg += fmt.Sprintf("%s: %s\r\n", k, headers.Get(k))
	}

	msg += "\r\n" + body

	err = smtp.SendMail(via.fwdViaSMTP.Address, auth, from, via.fwdViaSMTP.To, []byte(msg))
	return err
}

// fwdHTTPAPIEmailReq prepares a request to be sent by a HTTP API. It breaks the forwarding
// email down to it's various parts then allows for them to be passed through a template
// before passing the message on to be sent via HTTP API
func fwdHTTPAPIEmailReq(via fwdVia, from, subject, body string, headers mail.Header, isTest bool) (*http.Request, error) {
	var html string
	var text = body
	var useMultipart bool

	if isTest {
		if via.fwdViaHTTPAPI.From != "" {
			from = via.fwdViaHTTPAPI.From
		}
		if via.fwdViaHTTPAPI.Subject != "" {
			subject = via.fwdViaHTTPAPI.Subject
		}
		if via.fwdViaHTTPAPI.Text != "" {
			text = via.fwdViaHTTPAPI.Text
		}
		if via.fwdViaHTTPAPI.HTML != "" {
			html = via.fwdViaHTTPAPI.HTML
		}
	} else {
		contentType, params, _ := mime.ParseMediaType(headers.Get("Content-Type"))
		if strings.ToLower(contentType) == "multipart/alternative" {
			r := strings.NewReader(body)
			if boundary, ok := params["boundary"]; ok {
				buf := new(bytes.Buffer)
				mr := multipart.NewReader(r, boundary)
				for {
					p, err := mr.NextPart()
					if err == io.EOF {
						break
					}
					if err != nil {
						log.Warnf("mime: read next part: %v", err)
						continue
					}
					_, err = buf.ReadFrom(p)
					if err != nil {
						log.Warnf("mime: read from: %v", err)
						continue
					}
					headerContentType, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
					switch headerContentType {
					case "text/plain":
						text = buf.String()
					case "text/html":
						html = buf.String()
					}
					buf.Reset()
				}
			}
		}
	}

	Data := struct {
		From, Subject, Text, HTML string
	}{
		From:    from,
		Subject: subject,
		Text:    text,
		HTML:    html,
	}

	u, err := url.Parse(via.fwdViaHTTPAPI.URL())
	if err != nil {
		return nil, err
	}

	req := &http.Request{
		Method:     strings.ToUpper(via.fwdViaHTTPAPI.Method()),
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Host:       u.Host,
	}

	if via.fwdViaHTTPAPI.User() != nil && via.fwdViaHTTPAPI.Pass() != nil {
		req.SetBasicAuth(*via.fwdViaHTTPAPI.User(), *via.fwdViaHTTPAPI.Pass())
	}

	var buf = new(bytes.Buffer)
	for k, v := range via.fwdViaHTTPAPI.Headers() {
		if strings.ToLower(k) == "content-type" {
			contentType, _, _ := mime.ParseMediaType(v)
			useMultipart = contentType == "multipart/form-data"
		}
		buf.Reset()
		tmpl, err := template.New(k).Parse(v)
		if err != nil {
			return nil, fmt.Errorf("header template parse: %v", err)
		}
		err = tmpl.Execute(buf, Data)
		if err != nil {
			return nil, fmt.Errorf("header template exec: %v", err)
		}
		req.Header.Add(k, buf.String())
	}
	buf.Reset()

	var w *multipart.Writer
	var wb = new(bytes.Buffer)
	if useMultipart {
		w = multipart.NewWriter(wb)
		req.Header.Set("Content-Type", w.FormDataContentType())
	}
	var q url.URL
	for k, vv := range via.fwdViaHTTPAPI.Parameters() {
		for _, v := range vv {
			buf.Reset()
			tmpl, err := template.New(k).Parse(v)
			if err != nil {
				return nil, fmt.Errorf("param template parse: %v", err)
			}
			err = tmpl.Execute(buf, Data)
			if err != nil {
				return nil, fmt.Errorf("param template exec: %v", err)
			}
			if useMultipart {
				w.WriteField(k, buf.String())
				continue
			}
			q.Query().Add(k, buf.String())
		}
	}
	if useMultipart {
		w.Close()
		req.Body = ioutil.NopCloser(wb)
	} else if via.fwdViaHTTPAPI.Method() == http.MethodGet {
		req.URL.RawQuery = q.RawQuery
	} else {
		req.Body = ioutil.NopCloser(strings.NewReader(q.Query().Encode()))
	}

	return req, nil
}

// fwdHTTPAPIEmail is the function that sends email via HTTP-API if a HTTP-API version has been defined
func fwdHTTPAPIEmail(via fwdVia, from, subject, body string, headers mail.Header, isTest bool) (err error) {
	var client http.Client

	req, err := fwdHTTPAPIEmailReq(via, from, subject, body, headers, isTest)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid response code: %d", resp.StatusCode)
	}

	return nil
}
