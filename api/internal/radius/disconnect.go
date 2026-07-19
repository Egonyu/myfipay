// Package radius implements the client side of RFC 5176 Dynamic Authorization
// (Disconnect-Request) — enough to kick a live hotspot session off a MikroTik
// NAS when an operator terminates it. The wizard script already configures
// routers with `/radius incoming set accept=yes` (UDP 3799).
package radius

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

const (
	codeDisconnectRequest = 40
	codeDisconnectACK     = 41
	codeDisconnectNAK     = 42

	attrUserName         = 1
	attrCallingStationID = 31
	attrAcctSessionID    = 44

	// DynAuthPort is the standard RFC 5176 port (RouterOS default).
	DynAuthPort = 3799

	attempts       = 2
	attemptTimeout = 1500 * time.Millisecond
)

// Session identifies the NAS session to tear down. Empty fields are omitted
// from the request; the NAS matches on every attribute present.
type Session struct {
	Username         string
	AcctSessionID    string
	CallingStationID string
}

type avp struct {
	typ   byte
	value []byte
}

func encodeAttrs(attrs []avp) ([]byte, error) {
	var out []byte
	for _, a := range attrs {
		if len(a.value) == 0 {
			continue
		}
		if len(a.value) > 253 {
			return nil, fmt.Errorf("attribute %d too long (%d bytes)", a.typ, len(a.value))
		}
		out = append(out, a.typ, byte(2+len(a.value)))
		out = append(out, a.value...)
	}
	return out, nil
}

// buildDisconnect returns the wire packet. The Request Authenticator is
// MD5(Code + Identifier + Length + 16 zero octets + Attributes + Secret).
func buildDisconnect(id byte, secret string, s Session) ([]byte, error) {
	attrs, err := encodeAttrs([]avp{
		{attrUserName, []byte(s.Username)},
		{attrAcctSessionID, []byte(s.AcctSessionID)},
		{attrCallingStationID, []byte(s.CallingStationID)},
	})
	if err != nil {
		return nil, err
	}

	length := 20 + len(attrs)
	pkt := make([]byte, length)
	pkt[0] = codeDisconnectRequest
	pkt[1] = id
	binary.BigEndian.PutUint16(pkt[2:4], uint16(length))
	// pkt[4:20] left zero for the authenticator hash
	copy(pkt[20:], attrs)

	h := md5.New()
	h.Write(pkt)
	h.Write([]byte(secret))
	copy(pkt[4:20], h.Sum(nil))
	return pkt, nil
}

// validResponse checks code/id and the Response Authenticator:
// MD5(Code + Identifier + Length + Request Authenticator + Attributes + Secret).
func validResponse(resp, reqAuth []byte, id byte, secret string) (acked bool, err error) {
	if len(resp) < 20 {
		return false, fmt.Errorf("short packet (%d bytes)", len(resp))
	}
	length := int(binary.BigEndian.Uint16(resp[2:4]))
	if length < 20 || length > len(resp) {
		return false, fmt.Errorf("bad length field %d", length)
	}
	resp = resp[:length]
	if resp[1] != id {
		return false, fmt.Errorf("identifier mismatch")
	}
	if resp[0] != codeDisconnectACK && resp[0] != codeDisconnectNAK {
		return false, fmt.Errorf("unexpected code %d", resp[0])
	}

	h := md5.New()
	h.Write(resp[0:4])
	h.Write(reqAuth)
	h.Write(resp[20:])
	h.Write([]byte(secret))
	if subtle.ConstantTimeCompare(h.Sum(nil), resp[4:20]) != 1 {
		return false, fmt.Errorf("response authenticator invalid (wrong secret?)")
	}
	return resp[0] == codeDisconnectACK, nil
}

// Disconnect sends a Disconnect-Request to addr (host or host:port; default
// port 3799) and waits for the ACK/NAK. Returns (true, nil) on ACK,
// (false, nil) on NAK, and (false, err) if the NAS never answered validly.
func Disconnect(ctx context.Context, addr, secret string, s Session) (bool, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, fmt.Sprintf("%d", DynAuthPort))
	}

	var idBuf [1]byte
	if _, err := rand.Read(idBuf[:]); err != nil {
		return false, err
	}
	id := idBuf[0]

	pkt, err := buildDisconnect(id, secret, s)
	if err != nil {
		return false, err
	}
	reqAuth := pkt[4:20]

	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", addr)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	buf := make([]byte, 4096)
	var lastErr error
	for i := 0; i < attempts; i++ {
		deadline := time.Now().Add(attemptTimeout)
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
		conn.SetDeadline(deadline)

		if _, err := conn.Write(pkt); err != nil {
			lastErr = err
			continue
		}
		n, err := conn.Read(buf)
		if err != nil {
			lastErr = err
			continue
		}
		acked, err := validResponse(buf[:n], reqAuth, id, secret)
		if err != nil {
			lastErr = err
			continue
		}
		return acked, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no response")
	}
	return false, fmt.Errorf("disconnect %s: %w", addr, lastErr)
}
