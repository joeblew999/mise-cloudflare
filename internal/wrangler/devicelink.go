package wrangler

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
)

var (
	verifyURL = regexp.MustCompile(`https://\S+/device/verify\S*`)
	userCode  = regexp.MustCompile(`^\s*([A-Za-z0-9-]{6,12})\s*$`)
)

// DeviceLink copies a device login's output (wrangler's or cf's) to w, and once it has printed the link and the code to
// type there, adds the link with the code already in it (?user_code=, RFC 8628's verification_uri_complete): opened on
// a phone, it needs only Allow. They print the two apart when no browser opens.
func DeviceLink(r io.Reader, w io.Writer) {
	sc := bufio.NewScanner(r)
	link, done := "", false
	for sc.Scan() {
		line := sc.Text()
		fmt.Fprintln(w, line)
		if done {
			continue
		}
		if m := verifyURL.FindString(line); m != "" {
			link = m
		} else if m := userCode.FindStringSubmatch(line); m != nil && link != "" && !strings.Contains(link, "user_code=") {
			u, err := url.Parse(link)
			if err == nil {
				q := u.Query()
				q.Set("user_code", m[1])
				u.RawQuery = q.Encode()
				fmt.Fprintf(w, "\nOr open this link, the code already in it, on any device, and click Allow:\n\n  %s\n\n", u)
				done = true
			}
		}
	}
}
