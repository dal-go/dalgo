package end2end

import (
	"regexp"
	"testing"
)

var reVersion = regexp.MustCompile(`\d.\d.\d+(-.+)?`)

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("Version is empty")
	}
	if !reVersion.MatchString(Version) {
		t.Fatalf("Version is not matching expected pattern %v: %v", reVersion.String(), Version)
	}
}
