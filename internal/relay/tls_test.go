package relay

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"testing"
)

func TestParseCraftedClientHello(t *testing.T) {
	msg := buildClientHello("www.example.com")
	raw := append(wrapRecord(msg[:10]), wrapRecord(msg[10:])...)
	sni, got, err := readClientHello(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if sni != "www.example.com" {
		t.Fatal(sni)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("raw bytes were not preserved")
	}
}

func TestParseRealClientHello(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			got <- err.Error()
			return
		}
		defer c.Close()
		sni, _, err := readClientHello(c)
		if err != nil {
			got <- err.Error()
			return
		}
		got <- sni
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = tls.Client(conn, &tls.Config{ServerName: "cdn.example.com", InsecureSkipVerify: true}).Handshake()
	if sni := <-got; sni != "cdn.example.com" {
		t.Fatal(sni)
	}
}

func TestRejectPlaintext(t *testing.T) {
	_, _, err := readClientHello(bytes.NewReader([]byte("GET / HTTP/1.1\r\n\r\n")))
	if err == nil {
		t.Fatal("accepted plaintext")
	}
}

func buildClientHello(serverName string) []byte {
	name := []byte(serverName)
	sni := make([]byte, 5+len(name))
	listLen := 3 + len(name)
	sni[0] = byte(listLen >> 8)
	sni[1] = byte(listLen)
	sni[2] = 0
	sni[3] = byte(len(name) >> 8)
	sni[4] = byte(len(name))
	copy(sni[5:], name)

	ext := make([]byte, 4+len(sni))
	ext[2] = byte(len(sni) >> 8)
	ext[3] = byte(len(sni))
	copy(ext[4:], sni)

	body := []byte{0x03, 0x03}
	body = append(body, make([]byte, 32)...)
	body = append(body, 0)
	body = append(body, 0x00, 0x02, 0x00, 0x2f)
	body = append(body, 1, 0)
	body = append(body, byte(len(ext)>>8), byte(len(ext)))
	body = append(body, ext...)

	msg := make([]byte, 4+len(body))
	msg[0] = 1
	msg[1] = byte(len(body) >> 16)
	msg[2] = byte(len(body) >> 8)
	msg[3] = byte(len(body))
	copy(msg[4:], body)
	return msg
}

func wrapRecord(fragment []byte) []byte {
	rec := make([]byte, 5+len(fragment))
	rec[0] = 22
	rec[1] = 3
	rec[2] = 1
	rec[3] = byte(len(fragment) >> 8)
	rec[4] = byte(len(fragment))
	copy(rec[5:], fragment)
	return rec
}

func TestHostHeader(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n", "www.example.com"},
		{"GET / HTTP/1.1\r\nhost: www.example.com:80\r\n\r\n", "www.example.com"},
		{"GET http://example.com/a HTTP/1.1\r\n\r\n", "example.com"},
		{"GET / HTTP/1.1\r\nHost: [2001:db8::1]:80\r\n\r\n", "2001:db8::1"},
	}
	for _, tc := range cases {
		got, err := hostFromHeaders([]byte(tc.in))
		if err != nil || got != tc.want {
			t.Fatalf("%q -> %q %v", tc.in, got, err)
		}
	}
	if _, err := hostFromHeaders([]byte("GET / HTTP/1.1\r\n\r\n")); err == nil {
		t.Fatal("missing host")
	}
}

func TestReadHeaderKeepsBody(t *testing.T) {
	raw := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 4\r\n\r\nping"
	br := bufio.NewReader(bytes.NewReader([]byte(raw)))
	hdr, err := readHeaderBlock(br, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(hdr, []byte("\r\n\r\n")) {
		t.Fatalf("header %q", hdr)
	}
	body, err := io.ReadAll(br)
	if err != nil || string(body) != "ping" {
		t.Fatalf("body %q %v", body, err)
	}
}
