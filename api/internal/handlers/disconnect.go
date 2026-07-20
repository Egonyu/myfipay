package handlers

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/myfibase/myfibase/internal/radius"
)

// disconnectResult reports what happened when we told a NAS to drop the
// user's live session (RFC 5176). Terminate is best-effort: revoking
// radcheck always succeeds; the kick can fail if the router is unreachable.
type disconnectResult struct {
	NasIP string `json:"nas_ip"`
	Acked bool   `json:"acked"`
	Error string `json:"error,omitempty"`
}

// disconnectLiveSessions sends a Disconnect-Request for every open radacct
// row of this username, using each router's per-device secret from the nas
// table. Rows whose NAS is not a registered router (e.g. localhost radtest)
// are skipped.
func (h *Handler) disconnectLiveSessions(ctx context.Context, username string) []disconnectResult {
	rows, err := h.db.Query(ctx, `
		SELECT ra.acctsessionid, host(ra.nasipaddress), ra.callingstationid,
		       COALESCE(host(ra.framedipaddress), ''), COALESCE(n.secret, '')
		FROM radacct ra
		LEFT JOIN nas n ON n.nasname = host(ra.nasipaddress)
		WHERE ra.username = $1 AND ra.acctstoptime IS NULL
	`, username)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type target struct {
		sess   radius.Session
		nasIP  string
		secret string
	}
	var targets []target
	for rows.Next() {
		var t target
		var framedIP string
		if err := rows.Scan(&t.sess.AcctSessionID, &t.nasIP, &t.sess.CallingStationID, &framedIP, &t.secret); err == nil {
			t.sess.Username = username
			// RouterOS 7.16 hotspot NAKs (406) any Disconnect-Request that
			// lacks the client's Framed-IP-Address, whatever else matches.
			t.sess.FramedIP = net.ParseIP(framedIP)
			targets = append(targets, t)
		}
	}

	var results []disconnectResult
	for _, t := range targets {
		res := disconnectResult{NasIP: t.nasIP}
		if t.secret == "" {
			res.Error = "no registered router for this NAS"
			results = append(results, res)
			continue
		}
		dctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		acked, err := radius.Disconnect(dctx, t.nasIP, t.secret, t.sess)
		cancel()
		res.Acked = acked
		if err != nil {
			res.Error = err.Error()
		}
		log.Printf("disconnect: user %s on %s acked=%v err=%v", username, t.nasIP, acked, err)
		results = append(results, res)
	}
	return results
}
