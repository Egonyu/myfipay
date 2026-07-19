package handlers

import (
	"strconv"
	"strings"
)

// Pure money math, extracted from the handlers so it is unit-testable without a
// database. Every function here must stay side-effect free.
//
// All amounts are integer UGX (no sub-unit currency in mobile money).
// Commission truncates toward zero: the platform/agent never rounds up its cut.

// platformCommission is the platform's cut of gross mobile-money revenue.
func platformCommission(grossUGX int, rate float64) int {
	return int(float64(grossUGX) * rate)
}

// operatorAvailable is what an operator can still withdraw:
// gross mobile-money minus platform commission minus everything already
// claimed by non-rejected payout requests. Never negative.
func operatorAvailable(grossUGX int, rate float64, withdrawnUGX int) int {
	available := grossUGX - platformCommission(grossUGX, rate) - withdrawnUGX
	if available < 0 {
		return 0
	}
	return available
}

// agentAvailable is what an agent can still withdraw: earned commissions minus
// non-rejected payout requests. Never negative.
func agentAvailable(earnedUGX, reservedUGX int) int {
	available := earnedUGX - reservedUGX
	if available < 0 {
		return 0
	}
	return available
}

// agentCommission is the referring agent's cut of one confirmed payment.
// Truncates below 1 UGX — free plans and sub-unit amounts earn no commission.
func agentCommission(amountUGX int, rate float64) int {
	c := int(float64(amountUGX) * rate)
	if c < 1 {
		return 0
	}
	return c
}

// parseCommissionRate validates a per-tenant commission-rate override
// (tenants.settings->>'commission_rate'). Empty, unparsable, or out-of-range
// values fall back to the platform default. 0 is valid (zero-fee tenant); 1 is
// not (the platform may not take everything).
func parseCommissionRate(s string, fallback float64) float64 {
	if s == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(s, 64)
	if err != nil || parsed < 0 || parsed >= 1 {
		return fallback
	}
	return parsed
}

// classifyZengapayEvent maps a webhook's transactionStatus/event pair to what
// we should do with the pending payment: "success", "failed", or "" (ignore).
// Sandbox signals via event name; production may signal via transactionStatus —
// both are matched case-insensitively.
func classifyZengapayEvent(transactionStatus, event string) string {
	statusUpper := strings.ToUpper(transactionStatus)
	eventUpper := strings.ToUpper(event)
	switch {
	case statusUpper == "SUCCEEDED" || statusUpper == "SUCCESSFUL" || statusUpper == "COMPLETED" ||
		eventUpper == "COLLECTION.SUCCESS":
		return "success"
	case statusUpper == "FAILED" || statusUpper == "FAILURE" || eventUpper == "COLLECTION.FAILED":
		return "failed"
	}
	return ""
}
