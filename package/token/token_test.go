package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestIssueVerify(t *testing.T) {
	m := NewManager("0123456789abcdef0123456789abcdef", time.Minute)
	raw, err := m.Issue("user-1", "user")
	if err != nil {
		t.Fatal(err)
	}
	c, err := m.Verify(raw)
	if err != nil || c.UserID != "user-1" || c.Role != "user" {
		t.Fatalf("claims %+v err %v", c, err)
	}

	other := NewManager("another-secret-another-secret-xx", time.Minute)
	if _, err := other.Verify(raw); err == nil {
		t.Fatal("token signed with another secret must fail")
	}

	expired := NewManager("0123456789abcdef0123456789abcdef", time.Minute)
	expired.now = func() time.Time { return time.Now().Add(-time.Hour) }
	old, _ := expired.Issue("user-1", "user")
	if _, err := m.Verify(old); err == nil {
		t.Fatal("expired token must fail")
	}

	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "x", "iss": issuer, "exp": time.Now().Add(time.Hour).Unix()}).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := m.Verify(none); err == nil {
		t.Fatal("alg=none must be rejected")
	}
}
