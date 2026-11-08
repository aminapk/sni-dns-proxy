package relay

import (
	"bytes"
	"errors"
	"io"
)

func readClientHello(r io.Reader) (string, []byte, error) {
	var raw bytes.Buffer
	var hs bytes.Buffer
	hsLen := -1
	for nrec := 0; nrec < 8 && (hsLen < 0 || hs.Len() < hsLen); nrec++ {
		header := make([]byte, 5)
		if _, err := io.ReadFull(r, header); err != nil {
			return "", nil, err
		}
		if header[0] != 22 {
			return "", nil, errors.New("not a TLS handshake")
		}
		n := int(header[3])<<8 | int(header[4])
		if n <= 0 || n > 16384 {
			return "", nil, errors.New("invalid TLS record length")
		}
		frag := make([]byte, n)
		if _, err := io.ReadFull(r, frag); err != nil {
			return "", nil, err
		}
		raw.Write(header)
		raw.Write(frag)
		hs.Write(frag)
		if hsLen < 0 && hs.Len() >= 4 {
			if hs.Bytes()[0] != 1 {
				return "", nil, errors.New("not a ClientHello")
			}
			hsLen = 4 + (int(hs.Bytes()[1])<<16 | int(hs.Bytes()[2])<<8 | int(hs.Bytes()[3]))
			if hsLen < 4 || hsLen > 20000 {
				return "", nil, errors.New("ClientHello is an unexpected size")
			}
		}
	}
	if hsLen < 0 || hs.Len() < hsLen {
		return "", nil, errors.New("truncated ClientHello")
	}
	sni, err := parseSNI(hs.Bytes()[:hsLen])
	if err != nil {
		return "", raw.Bytes(), err
	}
	return sni, raw.Bytes(), nil
}

func parseSNI(msg []byte) (string, error) {
	if len(msg) < 4 || msg[0] != 1 {
		return "", errors.New("not a ClientHello")
	}
	hl := int(msg[1])<<16 | int(msg[2])<<8 | int(msg[3])
	if hl < 0 || 4+hl > len(msg) {
		return "", errors.New("truncated ClientHello")
	}
	body := msg[4 : 4+hl]
	if len(body) < 35 {
		return "", errors.New("ClientHello is too short")
	}
	i := 34 // version + random
	sidLen := int(body[i])
	i += 1 + sidLen
	if i+2 > len(body) {
		return "", errors.New("truncated ClientHello")
	}
	csLen := int(body[i])<<8 | int(body[i+1])
	i += 2 + csLen
	if i+1 > len(body) {
		return "", errors.New("truncated ClientHello")
	}
	compLen := int(body[i])
	i += 1 + compLen
	if i+2 > len(body) {
		return "", errors.New("no SNI")
	}
	extLen := int(body[i])<<8 | int(body[i+1])
	i += 2
	if extLen < 0 || i+extLen > len(body) {
		return "", errors.New("truncated extensions")
	}
	end := i + extLen
	for i+4 <= end {
		typ := int(body[i])<<8 | int(body[i+1])
		el := int(body[i+2])<<8 | int(body[i+3])
		i += 4
		if el < 0 || i+el > end {
			return "", errors.New("truncated extension")
		}
		data := body[i : i+el]
		i += el
		if typ == 0 {
			return serverName(data)
		}
	}
	return "", errors.New("no SNI")
}

func serverName(data []byte) (string, error) {
	if len(data) < 2 {
		return "", errors.New("empty SNI")
	}
	listLen := int(data[0])<<8 | int(data[1])
	if listLen < 0 || 2+listLen > len(data) {
		return "", errors.New("truncated SNI")
	}
	j := 2
	end := 2 + listLen
	for j+3 <= end {
		typ := data[j]
		nlen := int(data[j+1])<<8 | int(data[j+2])
		j += 3
		if nlen < 0 || j+nlen > end {
			return "", errors.New("truncated SNI name")
		}
		if typ == 0 {
			if nlen == 0 || nlen > 253 {
				return "", errors.New("SNI length is invalid")
			}
			return string(data[j : j+nlen]), nil
		}
		j += nlen
	}
	return "", errors.New("no host name")
}
