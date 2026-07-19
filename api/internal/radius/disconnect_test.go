package radius

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// fakeNAS answers Disconnect-Requests on a loopback UDP socket. It verifies
// the Request Authenticator against secret and replies with respCode; if
// wrongSecret is set the response authenticator is computed with it instead.
func fakeNAS(t *testing.T, secret string, respCode byte, wrongSecret string) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })

	go func() {
		buf := make([]byte, 4096)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			req := buf[:n]
			if len(req) < 20 || req[0] != codeDisconnectRequest {
				continue
			}

			// Verify request authenticator: MD5(pkt with zeroed auth + secret)
			chk := make([]byte, len(req))
			copy(chk, req)
			for i := 4; i < 20; i++ {
				chk[i] = 0
			}
			h := md5.New()
			h.Write(chk)
			h.Write([]byte(secret))
			want := h.Sum(nil)
			for i := range want {
				if want[i] != req[4+i] {
					return // bad authenticator: stay silent, client should time out
				}
			}

			resp := make([]byte, 20)
			resp[0] = respCode
			resp[1] = req[1]
			binary.BigEndian.PutUint16(resp[2:4], 20)
			respSecret := secret
			if wrongSecret != "" {
				respSecret = wrongSecret
			}
			h = md5.New()
			h.Write(resp[0:4])
			h.Write(req[4:20])
			h.Write([]byte(respSecret))
			copy(resp[4:20], h.Sum(nil))
			pc.WriteTo(resp, from)
		}
	}()
	return pc.LocalAddr().String()
}

var testSession = Session{
	Username:         "256700000001",
	AcctSessionID:    "81f00001",
	CallingStationID: "AA:BB:CC:DD:EE:FF",
}

func TestDisconnectACK(t *testing.T) {
	addr := fakeNAS(t, "s3cret", codeDisconnectACK, "")
	acked, err := Disconnect(context.Background(), addr, "s3cret", testSession)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !acked {
		t.Fatal("expected ACK")
	}
}

func TestDisconnectNAK(t *testing.T) {
	addr := fakeNAS(t, "s3cret", codeDisconnectNAK, "")
	acked, err := Disconnect(context.Background(), addr, "s3cret", testSession)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acked {
		t.Fatal("expected NAK, got ACK")
	}
}

func TestDisconnectWrongSecretTimesOut(t *testing.T) {
	// NAS with a different secret silently drops our request (authenticator
	// check fails on its side) — the client must report an error, not an ACK.
	addr := fakeNAS(t, "other-secret", codeDisconnectACK, "")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := Disconnect(ctx, addr, "s3cret", testSession); err == nil {
		t.Fatal("expected error when NAS secret differs")
	}
}

func TestDisconnectForgedResponseRejected(t *testing.T) {
	// NAS accepts our request but signs the ACK with the wrong secret —
	// the client must not treat it as a valid ACK.
	addr := fakeNAS(t, "s3cret", codeDisconnectACK, "forged")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := Disconnect(ctx, addr, "s3cret", testSession); err == nil {
		t.Fatal("expected error for forged response authenticator")
	}
}

func TestBuildDisconnectPacket(t *testing.T) {
	pkt, err := buildDisconnect(7, "s3cret", testSession)
	if err != nil {
		t.Fatal(err)
	}
	if pkt[0] != codeDisconnectRequest || pkt[1] != 7 {
		t.Fatalf("bad header: code=%d id=%d", pkt[0], pkt[1])
	}
	if got := int(binary.BigEndian.Uint16(pkt[2:4])); got != len(pkt) {
		t.Fatalf("length field %d != packet size %d", got, len(pkt))
	}
	// Empty attributes must be omitted entirely (zero-length AVPs are illegal)
	pkt2, err := buildDisconnect(7, "s3cret", Session{Username: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt2) != 20+3 {
		t.Fatalf("expected only User-Name attr, packet size %d", len(pkt2))
	}
}
