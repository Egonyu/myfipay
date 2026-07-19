package handlers

import "testing"

// The money invariants these tests protect:
//   - the platform never rounds its cut up (truncation, operator-favoring)
//   - balances can never go negative
//   - a bad per-tenant rate override can never zero out or invert the fee
//   - the 8% platform / 3% agent published rates produce the exact figures
//     shown to operators and agents

func TestPlatformCommission(t *testing.T) {
	cases := []struct {
		name  string
		gross int
		rate  float64
		want  int
	}{
		{"8% of 100k", 100_000, 0.08, 8_000},
		{"8% of 500 (one 1h plan)", 500, 0.08, 40},
		{"truncates, never rounds up", 1_999, 0.08, 159}, // 159.92 → 159
		{"zero gross", 0, 0.08, 0},
		{"zero rate (zero-fee tenant)", 100_000, 0, 0},
		{"gross below a shilling of fee", 10, 0.08, 0},
		{"large month: 50M UGX", 50_000_000, 0.08, 4_000_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := platformCommission(c.gross, c.rate); got != c.want {
				t.Errorf("platformCommission(%d, %v) = %d, want %d", c.gross, c.rate, got, c.want)
			}
		})
	}
}

func TestOperatorAvailable(t *testing.T) {
	cases := []struct {
		name      string
		gross     int
		rate      float64
		withdrawn int
		want      int
	}{
		{"nothing withdrawn", 100_000, 0.08, 0, 92_000},
		{"partial withdrawal", 100_000, 0.08, 50_000, 42_000},
		{"exactly drained", 100_000, 0.08, 92_000, 0},
		{"over-withdrawn floors at zero", 100_000, 0.08, 95_000, 0},
		{"no revenue", 0, 0.08, 0, 0},
		{"withdrawals but no revenue floors at zero", 0, 0.08, 10_000, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := operatorAvailable(c.gross, c.rate, c.withdrawn); got != c.want {
				t.Errorf("operatorAvailable(%d, %v, %d) = %d, want %d",
					c.gross, c.rate, c.withdrawn, got, c.want)
			}
		})
	}
}

func TestAgentAvailable(t *testing.T) {
	cases := []struct {
		name             string
		earned, reserved int
		want             int
	}{
		{"nothing reserved", 30_000, 0, 30_000},
		{"partially reserved", 30_000, 10_000, 20_000},
		{"fully reserved", 30_000, 30_000, 0},
		{"over-reserved floors at zero", 30_000, 40_000, 0},
		{"nothing earned", 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := agentAvailable(c.earned, c.reserved); got != c.want {
				t.Errorf("agentAvailable(%d, %d) = %d, want %d", c.earned, c.reserved, got, c.want)
			}
		})
	}
}

func TestAgentCommission(t *testing.T) {
	cases := []struct {
		name   string
		amount int
		rate   float64
		want   int
	}{
		{"3% of 500 (1h plan)", 500, 0.03, 15},
		{"3% of 2000 (day plan)", 2_000, 0.03, 60},
		{"3% of 8000 (week plan)", 8_000, 0.03, 240},
		{"truncates: 3% of 999 is 29 not 30", 999, 0.03, 29},
		{"below 1 UGX earns nothing", 20, 0.03, 0}, // 0.6 → 0
		{"free plan earns nothing", 0, 0.03, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := agentCommission(c.amount, c.rate); got != c.want {
				t.Errorf("agentCommission(%d, %v) = %d, want %d", c.amount, c.rate, got, c.want)
			}
		})
	}
}

func TestParseCommissionRate(t *testing.T) {
	const fallback = 0.08
	cases := []struct {
		name string
		in   string
		want float64
	}{
		{"empty falls back", "", fallback},
		{"valid override", "0.10", 0.10},
		{"zero is a valid zero-fee tenant", "0", 0},
		{"garbage falls back", "abc", fallback},
		{"negative falls back", "-0.1", fallback},
		{"1 falls back (platform may not take everything)", "1", fallback},
		{"above 1 falls back", "1.5", fallback},
		{"high but legal", "0.99", 0.99},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseCommissionRate(c.in, fallback); got != c.want {
				t.Errorf("parseCommissionRate(%q, %v) = %v, want %v", c.in, fallback, got, c.want)
			}
		})
	}
}

// Guard the published constants themselves: changing 8%/3% or the payout floor
// is a business decision (BUSINESS_MODEL.md), not a refactor side effect.
func TestPublishedRates(t *testing.T) {
	if defaultCommissionRate != 0.08 {
		t.Errorf("platform commission rate changed: %v (published rate is 8%%)", defaultCommissionRate)
	}
	if agentCommissionRate != 0.03 {
		t.Errorf("agent commission rate changed: %v (published rate is 3%%)", agentCommissionRate)
	}
	if minPayoutUGX != 5000 {
		t.Errorf("minimum payout changed: %d (published floor is UGX 5,000)", minPayoutUGX)
	}
}
