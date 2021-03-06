/*
Copyright 2013 WebFist AUTHORS

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webfist

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/mail"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// MaxEmailSize is the maxium size of an RFC 822 email, including
// both its headers and body.
const MaxEmailSize = 64 << 10

// Email wraps a signed email.
type Email struct {
	all  []byte
	msg  *mail.Message
	body []byte

	encSHA1 string // Lazy
}

// NewEmail parses all as an email and returns a wrapper around it.
// Its size and format is done, but no signing verification is done.
func NewEmail(all []byte) (*Email, error) {
	if len(all) > MaxEmailSize {
		return nil, errors.New("email too large")
	}
	msg, err := mail.ReadMessage(bytes.NewReader(all))
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(msg.Body)
	if err != nil {
		return nil, err
	}

	// TODO: Extract the message receive time for sorting purposes
	e := &Email{
		all:  all,
		msg:  msg,
		body: body,
	}
	return e, nil
}

// Verify returns whether
func (e *Email) Verify() bool {
	dkimVerifyOnce.Do(initDKIMVerify)
	if e.msg.Header.Get("DKIM-Signature") == "" {
		return false
	}

	cmd := exec.Command(dkimVerifyPath)
	cmd.Stdin = bytes.NewReader(e.all)
	out, err := cmd.CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) == "signature ok" {
		return true
	}
	return false
}

func (e *Email) From() (*EmailAddr, error) {
	mailAddr, err := mail.ParseAddress(e.msg.Header.Get("From"))
	if err != nil {
		return nil, err
	}
	return NewEmailAddr(mailAddr.Address), nil
}

func (e *Email) Date() (time.Time, error) {
	t, err := e.msg.Header.Date()
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// EncSHA1 returns a lowercase SHA1 hex of the encrypted email.
func (e *Email) EncSHA1() (string, error) {
	if e.encSHA1 != "" {
		return e.encSHA1, nil
	}
	r, err := e.Encrypted()
	if err != nil {
		return "", err
	}
	s1 := sha1.New()
	_, err = io.Copy(s1, r)
	if err != nil {
		return "", nil
	}

	e.encSHA1 = fmt.Sprintf("%x", s1.Sum(nil))
	return e.encSHA1, nil
}

func (e *Email) SetEncSHA1(x string) {
	e.encSHA1 = x
}

func (e *Email) Encrypted() (io.Reader, error) {
	addr, err := e.From()
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	ew := addr.Encrypter(pw)
	go func() {
		_, err := ew.Write(e.all)
		pw.CloseWithError(err)
	}()
	return pr, nil
}

//webfist=http://example.com/myjrd.json
var (
	assignmentRe = regexp.MustCompile(`\bwebfist\s*=\s*(\S+)`)
)

// WebFist returns the delegation identifier parse from the email. The email
// must contain a single assignment where the delegated WebFinger server lives.
//   webfist = http://example.com/my-profile.json
func (e *Email) WebFist() (string, error) {
	for _, match := range assignmentRe.FindAllSubmatch(e.body, -1) {
		if len(match) != 2 {
			continue
		}
		return string(match[1]), nil
	}
	return "", errors.New("'webfist' assignment missing")
}

var (
	dkimVerifyOnce sync.Once
	dkimVerifyPath string
)

const dkimFailMessage = "dkimverify / dkimverify.py not found. Install python-dkim (http://hewgill.com/pydkim/)"

func initDKIMVerify() {
	path, err := findDKIMVerify()
	if err != nil {
		log.Fatalf(dkimFailMessage)
	}
	dkimVerifyPath = path
}

func findDKIMVerify() (path string, err error) {
	for _, name := range []string{"dkimverify.py", "dkimverify"} {
		path, err = exec.LookPath(name)
		if err == nil {
			break
		}
	}
	return
}
