package auth

import (
	"strings"
	"testing"
	"time"

	"fulfillment/internal/crypt"
)

// RFC 6238 appendix B (SHA-1 secret "12345678901234567890"); the RFC lists 8 digits, we use the last 6.
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestTOTPMatchesRFC6238Vectors(t *testing.T) {
	for _, c := range []struct {
		unix int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		if got := TOTPCodeAt(rfcSecret, time.Unix(c.unix, 0)); got != c.want {
			t.Errorf("t=%d: %s, want %s", c.unix, got, c.want)
		}
	}
}

func TestCheckTOTPWindowAndReplay(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cur := now.Unix() / 30
	code := TOTPCodeAt(rfcSecret, now)

	step, ok := CheckTOTP(rfcSecret, code, now, 0)
	if !ok || step != cur {
		t.Fatalf("current code: step %d ok %v", step, ok)
	}
	// typed with a space or a hyphen
	if _, ok := CheckTOTP(rfcSecret, code[:3]+" "+code[3:], now, 0); !ok {
		t.Error("a code typed as '123 456' must work")
	}
	// the same code again, or an older one, is a replay
	if _, ok := CheckTOTP(rfcSecret, code, now, step); ok {
		t.Error("a used code must not work twice")
	}
	// one step of drift either way is fine, two is not
	if _, ok := CheckTOTP(rfcSecret, TOTPCodeAt(rfcSecret, now.Add(-30*time.Second)), now, 0); !ok {
		t.Error("previous step should be accepted")
	}
	if _, ok := CheckTOTP(rfcSecret, TOTPCodeAt(rfcSecret, now.Add(30*time.Second)), now, 0); !ok {
		t.Error("next step should be accepted")
	}
	if _, ok := CheckTOTP(rfcSecret, TOTPCodeAt(rfcSecret, now.Add(-90*time.Second)), now, 0); ok {
		t.Error("a code from 3 steps ago must be refused")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef", "000000x"} {
		if _, ok := CheckTOTP(rfcSecret, bad, now, 0); ok {
			t.Errorf("%q must be refused", bad)
		}
	}
	if _, ok := CheckTOTP("not base32!", code, now, 0); ok {
		t.Error("a broken secret must never validate")
	}
}

func TestNewTOTPSecretAndURI(t *testing.T) {
	a, b := NewTOTPSecret(), NewTOTPSecret()
	if a == b || len(a) != 32 || strings.Contains(a, "=") {
		t.Errorf("secrets: %q %q", a, b)
	}
	if TOTPCodeAt(a, time.Now()) == "" {
		t.Error("a generated secret must be usable")
	}
	uri := TOTPURI("Keypost (pay.example.com)", "me@example.com", a)
	for _, want := range []string{"otpauth://totp/Keypost%20%28pay.example.com%29:me@example.com?", "secret=" + a, "algorithm=SHA1", "digits=6", "period=30", "issuer=Keypost+%28pay.example.com%29"} {
		if !strings.Contains(uri, want) {
			t.Errorf("uri lacks %q: %s", want, uri)
		}
	}
}

func TestRecoveryCodes(t *testing.T) {
	crypt.SetSecret("test-secret-0123456789")
	codes, hashes := NewRecoveryCodes(10)
	if len(codes) != 10 || len(hashes) != 10 {
		t.Fatalf("%d codes, %d hashes", len(codes), len(hashes))
	}
	seen := map[string]bool{}
	for i, c := range codes {
		if len(c) != 11 || c[5] != '-' || strings.ContainsAny(c, "0oO1lI") || seen[c] {
			t.Errorf("odd code %q", c)
		}
		seen[c] = true
		// case, hyphen and spaces do not matter when it is typed back
		for _, typed := range []string{c, strings.ToUpper(c), strings.ReplaceAll(c, "-", ""), " " + c + " "} {
			if HashRecoveryCode(typed) != hashes[i] {
				t.Errorf("%q must hash like %q", typed, c)
			}
		}
		if strings.Contains(hashes[i], c[:5]) {
			t.Error("the hash must not contain the code")
		}
	}
	if HashRecoveryCode("aaaaa-bbbbb") == HashRecoveryCode("aaaaa-bbbbc") {
		t.Error("different codes must hash differently")
	}
}
