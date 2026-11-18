package version

import (
	"regexp"
	"testing"
)

func TestVersionIsSemver(t *testing.T) {
	re := regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
	if v := Version(); !re.MatchString(v) {
		t.Fatalf("VERSION %q is not a semantic version (expected e.g. 0.1.0, no leading v)", v)
	}
	if s := String(); s == "" || s[:len(Version())] != Version() {
		t.Fatalf("String() = %q should start with the version", s)
	}
}
