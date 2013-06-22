package webfist

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"net/mail"
	"os/exec"
	"sync"
)

// MaxEmailSize is the maxium size of an RFC 822 email, including
// both its headers and body.
const MaxEmailSize = 16 << 10

// Email wraps a signed email.
type Email struct {
	all  []byte
	msg  *mail.Message
	body []byte
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

	cmd := exec.Command(dkimVerifyPath)
	cmd.Stdin = bytes.NewReader(e.all)
	out, err := cmd.CombinedOutput()
	if err == nil && string(out) == "signature ok" {
		return true
	}
	return false
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