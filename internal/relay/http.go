package relay

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/url"
	"strings"
)

func readHeaderBlock(br *bufio.Reader, max int) ([]byte, error) {
	var hdr bytes.Buffer
	for {
		line, err := br.ReadSlice('\n')
		if len(line) > 0 {
			if hdr.Len()+len(line) > max {
				return nil, errors.New("headers too large")
			}
			hdr.Write(line)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimRight(line, "\r\n")) == 0 {
			return hdr.Bytes(), nil
		}
	}
}

func hostFromHeaders(hdr []byte) (string, error) {
	lines := bytes.Split(hdr, []byte{'\n'})
	var fromRequest string
	if len(lines) > 0 {
		parts := bytes.Fields(lines[0])
		if len(parts) >= 2 {
			target := string(parts[1])
			lower := strings.ToLower(target)
			if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
				if u, err := url.Parse(target); err == nil {
					fromRequest = u.Hostname()
				}
			}
		}
	}
	var headerHost string
	for _, line := range lines {
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 || bytes.IndexByte(line, ':') < 0 {
			continue
		}
		if bytes.HasPrefix(bytes.ToLower(line), []byte("host:")) {
			headerHost = strings.TrimSpace(string(line[5:]))
		}
	}
	h := headerHost
	if h == "" {
		h = fromRequest
	}
	if h == "" {
		return "", errors.New("missing host")
	}
	return splitHost(h), nil
}

func splitHost(h string) string {
	if strings.HasPrefix(h, "[") {
		host, _, err := net.SplitHostPort(h)
		if err != nil {
			return strings.Trim(h, "[]")
		}
		return host
	}
	if host, _, err := net.SplitHostPort(h); err == nil && strings.Contains(h, ":") {
		return host
	}
	return h
}
