package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func sign(body []byte, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyHMAC(t *testing.T) {
	body := []byte(`{"event":"collection.success","data":{"amount":"500.00"}}`)
	secret := "test-webhook-secret"

	t.Run("valid signature passes", func(t *testing.T) {
		if !verifyHMAC(body, sign(body, []byte(secret)), secret) {
			t.Error("valid signature rejected")
		}
	})

	t.Run("uppercase signature passes", func(t *testing.T) {
		sig := sign(body, []byte(secret))
		if !verifyHMAC(body, "  "+toUpperHex(sig)+"  ", secret) {
			t.Error("uppercase/padded signature rejected")
		}
	})

	t.Run("tampered body fails", func(t *testing.T) {
		sig := sign(body, []byte(secret))
		tampered := []byte(`{"event":"collection.success","data":{"amount":"999.00"}}`)
		if verifyHMAC(tampered, sig, secret) {
			t.Error("tampered body accepted")
		}
	})

	t.Run("wrong secret fails", func(t *testing.T) {
		if verifyHMAC(body, sign(body, []byte("other-secret")), secret) {
			t.Error("signature from wrong secret accepted")
		}
	})

	t.Run("empty signature fails", func(t *testing.T) {
		if verifyHMAC(body, "", secret) {
			t.Error("empty signature accepted")
		}
	})

	t.Run("hex-encoded secret variant passes", func(t *testing.T) {
		// If the configured secret is a hex string, ZengaPay may have signed
		// with its decoded bytes — verifyHMAC must accept that too.
		hexSecret := "deadbeefcafe0123"
		rawKey, _ := hex.DecodeString(hexSecret)
		if !verifyHMAC(body, sign(body, rawKey), hexSecret) {
			t.Error("signature keyed with hex-decoded secret rejected")
		}
	})

	t.Run("garbage signature fails", func(t *testing.T) {
		if verifyHMAC(body, "not-a-hex-signature", secret) {
			t.Error("garbage signature accepted")
		}
	})
}

func toUpperHex(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'f' {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}

func TestClassifyZengapayEvent(t *testing.T) {
	cases := []struct {
		name          string
		status, event string
		want          string
	}{
		{"sandbox success event", "", "collection.success", "success"},
		{"sandbox failed event", "", "collection.failed", "failed"},
		{"production SUCCEEDED status", "SUCCEEDED", "", "success"},
		{"production SUCCESSFUL status", "successful", "", "success"},
		{"production COMPLETED status", "Completed", "", "success"},
		{"production FAILED status", "FAILED", "", "failed"},
		{"production FAILURE status", "failure", "", "failed"},
		{"mixed-case event", "", "Collection.Success", "success"},
		{"unknown status ignored", "PENDING", "", ""},
		{"unknown event ignored", "", "collection.pending", ""},
		{"both empty ignored", "", "", ""},
		// A failed status must win over stale/absent success signals and vice
		// versa — but if both somehow claim success and failure, success wins
		// (matches the original switch ordering; dedup makes replays moot).
		{"conflicting signals keep original precedence", "FAILED", "collection.success", "success"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyZengapayEvent(c.status, c.event); got != c.want {
				t.Errorf("classifyZengapayEvent(%q, %q) = %q, want %q", c.status, c.event, got, c.want)
			}
		})
	}
}

// The webhook payload arrives either nested ({"event","data":{...}}) or flat —
// both must decode to the same transaction fields the handler reads.
func TestZengapayPayloadDecoding(t *testing.T) {
	t.Run("nested data format (sandbox)", func(t *testing.T) {
		raw := []byte(`{
			"event": "collection.success",
			"data": {
				"transactionReference": "ZP-123",
				"transactionExternalReference": "pay-456",
				"transactionStatus": "SUCCEEDED",
				"amount": "500.00",
				"msisdn": "256700000001"
			}
		}`)
		var p zengapayWebhookPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		txn := p.Data
		if txn == nil {
			t.Fatal("nested data not decoded")
		}
		if txn.TransactionReference != "ZP-123" || txn.TransactionExternalReference != "pay-456" ||
			txn.Phone != "256700000001" {
			t.Errorf("nested fields wrong: %+v", txn)
		}
		if classifyZengapayEvent(txn.TransactionStatus, p.Event) != "success" {
			t.Error("nested payload not classified as success")
		}
	})

	t.Run("flat format (fallback)", func(t *testing.T) {
		raw := []byte(`{
			"transactionReference": "ZP-789",
			"transactionExternalReference": "pay-999",
			"transactionStatus": "FAILED",
			"msisdn": "256700000002"
		}`)
		var p zengapayWebhookPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		txn := p.Data
		if txn == nil {
			txn = &p.zengapayTxn
		}
		if txn.TransactionReference != "ZP-789" || txn.TransactionExternalReference != "pay-999" {
			t.Errorf("flat fields wrong: %+v", txn)
		}
		if classifyZengapayEvent(txn.TransactionStatus, p.Event) != "failed" {
			t.Error("flat payload not classified as failed")
		}
	})
}
