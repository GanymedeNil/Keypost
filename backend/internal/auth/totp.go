package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"fulfillment/internal/crypt"
)

// TOTP as in RFC 6238 with the parameters every authenticator app supports:
// HMAC-SHA1, 6 digits, 30 second steps.
const (
	totpDigits = 6
	totpPeriod = 30
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a fresh 160-bit secret in the base32 form apps expect.
func NewTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic("auth: no randomness: " + err.Error())
	}
	return b32.EncodeToString(b)
}

func hotp(key []byte, counter int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, bin%mod)
}

func decodeSecret(secret string) ([]byte, bool) {
	key, err := b32.DecodeString(strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", "")))
	return key, err == nil && len(key) > 0
}

// TOTPCodeAt is the code an authenticator shows at time t (used by tests).
func TOTPCodeAt(secret string, t time.Time) string {
	key, ok := decodeSecret(secret)
	if !ok {
		return ""
	}
	return hotp(key, t.Unix()/totpPeriod)
}

// NormalizeCode strips the spaces and hyphens people type into a code box.
func NormalizeCode(code string) string {
	return strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(code))
}

// CheckTOTP validates code against secret around now (one step of clock drift
// either way). A step at or before lastStep is refused so that a code, once
// used, cannot be replayed; on success the matched step is returned.
func CheckTOTP(secret, code string, now time.Time, lastStep int64) (step int64, ok bool) {
	code = NormalizeCode(code)
	key, good := decodeSecret(secret)
	if !good || len(code) != totpDigits {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	for s := cur - 1; s <= cur+1; s++ {
		if s <= lastStep {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotp(key, s)), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// TOTPURI is the otpauth:// link an authenticator app imports (usually from a QR code).
func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(totpPeriod))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// ---------------------------------------------------------------- recovery codes

// no 0/o/1/l/i: the codes are read off a screen or a piece of paper
const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// NewRecoveryCodes returns n codes ("xxxxx-xxxxx") and their stored hashes.
func NewRecoveryCodes(n int) (codes, hashes []string) {
	max := big.NewInt(int64(len(recoveryAlphabet)))
	for i := 0; i < n; i++ {
		b := make([]byte, 10)
		for j := range b {
			x, err := rand.Int(rand.Reader, max)
			if err != nil {
				panic("auth: no randomness: " + err.Error())
			}
			b[j] = recoveryAlphabet[x.Int64()]
		}
		code := string(b[:5]) + "-" + string(b[5:])
		codes = append(codes, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return codes, hashes
}

// HashRecoveryCode is the keyed hash stored for a recovery code (case and hyphen insensitive).
func HashRecoveryCode(code string) string {
	return crypt.MAC("mfa-recovery", strings.ToLower(NormalizeCode(code)))
}
