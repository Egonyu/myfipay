// Package radius implements the client side of RFC 5176 Dynamic Authorization
// (Disconnect-Request) — enough to kick a live hotspot session off a MikroTik
// NAS when an operator terminates it. The wizard script already configures
// routers with `/radius incoming set accept=yes` (UDP 3799).
package radius

import (
	"context"
	"crypto/hmac"
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

	attrUserName             = 1
	attrFramedIPAddress      = 8
	attrReplyMessage         = 18
	attrCallingStationID     = 31
	attrAcctSessionID        = 44
	attrMessageAuthenticator = 80
	attrErrorCause           = 101

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
	FramedIP         net.IP // hotspot client address; RouterOS needs it to key the teardown
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

// buildDisconnect returns the wire packet. A Message-Authenticator (RFC 3579)
// is always included — RouterOS with require-message-auth (the post-BlastRADIUS
// default) rejects Disconnect-Requests without one. Per RFC 5176 it is
// HMAC-MD5 over the packet with both the Request Authenticator field and its
// own value zeroed; the Request Authenticator is then
// MD5(Code + Identifier + Length + 16 zero octets + Attributes + Secret).
func buildDisconnect(id byte, secret string, s Session) ([]byte, error) {
	attrs, err := encodeAttrs([]avp{
		{attrUserName, []byte(s.Username)},
		{attrFramedIPAddress, []byte(s.FramedIP.To4())},
		{attrAcctSessionID, []byte(s.AcctSessionID)},
		{attrCallingStationID, []byte(s.CallingStationID)},
		{attrMessageAuthenticator, make([]byte, 16)},
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

	mac := hmac.New(md5.New, []byte(secret))
	mac.Write(pkt)
	copy(pkt[length-16:], mac.Sum(nil)) // Message-Authenticator is the last attribute

	h := md5.New()
	h.Write(pkt)
	h.Write([]byte(secret))
	copy(pkt[4:20], h.Sum(nil))
	return pkt, nil
}

// errorCauseNames maps RFC 5176 Error-Cause values to their standard names.
var errorCauseNames = map[uint32]string{
	401: "Unsupported Attribute",
	402: "Missing Attribute",
	403: "NAS Identification Mismatch",
	404: "Invalid Request",
	405: "Unsupported Service",
	406: "Unsupported Extension",
	407: "Invalid Attribute Value",
	501: "Administratively Prohibited",
	503: "Session Context Not Found",
	506: "Resources Unavailable",
}

// Result reports the NAS's answer to a Disconnect-Request. On a NAK,
// ErrorCause/ErrorCauseName and ReplyMessage carry the NAS's stated reason
// when it sent one (zero values otherwise).
type Result struct {
	Acked          bool
	ErrorCause     uint32
	ErrorCauseName string
	ReplyMessage   string
}

// parseNAKDetail extracts Error-Cause and Reply-Message from the attribute
// bytes of an already-validated response.
func parseNAKDetail(attrs []byte, r *Result) {
	for len(attrs) >= 2 {
		typ, l := attrs[0], int(attrs[1])
		if l < 2 || l > len(attrs) {
			return
		}
		val := attrs[2:l]
		switch typ {
		case attrErrorCause:
			if len(val) == 4 {
				r.ErrorCause = binary.BigEndian.Uint32(val)
				r.ErrorCauseName = errorCauseNames[r.ErrorCause]
			}
		case attrReplyMessage:
			r.ReplyMessage = string(val)
		}
		attrs = attrs[l:]
	}
}

// validResponse checks code/id and the Response Authenticator:
// MD5(Code + Identifier + Length + Request Authenticator + Attributes + Secret).
func validResponse(resp, reqAuth []byte, id byte, secret string) (res Result, err error) {
	if len(resp) < 20 {
		return res, fmt.Errorf("short packet (%d bytes)", len(resp))
	}
	length := int(binary.BigEndian.Uint16(resp[2:4]))
	if length < 20 || length > len(resp) {
		return res, fmt.Errorf("bad length field %d", length)
	}
	resp = resp[:length]
	if resp[1] != id {
		return res, fmt.Errorf("identifier mismatch")
	}
	if resp[0] != codeDisconnectACK && resp[0] != codeDisconnectNAK {
		return res, fmt.Errorf("unexpected code %d", resp[0])
	}

	h := md5.New()
	h.Write(resp[0:4])
	h.Write(reqAuth)
	h.Write(resp[20:])
	h.Write([]byte(secret))
	if subtle.ConstantTimeCompare(h.Sum(nil), resp[4:20]) != 1 {
		return res, fmt.Errorf("response authenticator invalid (wrong secret?)")
	}
	res.Acked = resp[0] == codeDisconnectACK
	if !res.Acked {
		parseNAKDetail(resp[20:], &res)
	}
	return res, nil
}

// Disconnect sends a Disconnect-Request to addr (host or host:port; default
// port 3799) and waits for the ACK/NAK. Returns (true, nil) on ACK,
// (false, nil) on NAK, and (false, err) if the NAS never answered validly.
func Disconnect(ctx context.Context, addr, secret string, s Session) (bool, error) {
	res, err := DisconnectDetail(ctx, addr, secret, s)
	return res.Acked, err
}

// DisconnectDetail is Disconnect but surfaces the NAK's Error-Cause and
// Reply-Message for diagnostics.
func DisconnectDetail(ctx context.Context, addr, secret string, s Session) (Result, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, fmt.Sprintf("%d", DynAuthPort))
	}

	var idBuf [1]byte
	if _, err := rand.Read(idBuf[:]); err != nil {
		return Result{}, err
	}
	id := idBuf[0]

	pkt, err := buildDisconnect(id, secret, s)
	if err != nil {
		return Result{}, err
	}
	reqAuth := pkt[4:20]

	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", addr)
	if err != nil {
		return Result{}, err
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
		res, err := validResponse(buf[:n], reqAuth, id, secret)
		if err != nil {
			lastErr = err
			continue
		}
		return res, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no response")
	}
	return Result{}, fmt.Errorf("disconnect %s: %w", addr, lastErr)
}
