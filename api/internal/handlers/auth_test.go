package handlers

import "testing"

func TestLoginLocked(t *testing.T) {
	cases := []struct {
		ipFails, acctFails int64
		want               bool
	}{
		{0, 0, false},
		{loginMaxPerIP - 1, 0, false},
		{loginMaxPerIP, 0, true},
		{0, loginMaxPerAccount - 1, false},
		{0, loginMaxPerAccount, true},
		{loginMaxPerIP, loginMaxPerAccount, true},
	}
	for _, c := range cases {
		if got := loginLocked(c.ipFails, c.acctFails); got != c.want {
			t.Errorf("loginLocked(%d, %d) = %v, want %v", c.ipFails, c.acctFails, got, c.want)
		}
	}
}
