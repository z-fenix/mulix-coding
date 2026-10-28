package sessionctx

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func ValidateIdentity(id Identity) error {
	if !id.Present {
		return nil
	}
	if id.Value == "" || len(id.Value) > 512 || !utf8.ValidString(id.Value) || strings.TrimSpace(id.Value) != id.Value {
		return &ResolveError{Kind: "identity-invalid", Err: fmt.Errorf("session ID must be nonempty UTF-8, at most 512 bytes, without surrounding whitespace")}
	}
	for _, r := range id.Value {
		if unicode.IsControl(r) {
			return &ResolveError{Kind: "identity-invalid", Err: fmt.Errorf("session ID contains control characters")}
		}
	}
	return nil
}

func ValidateChange(change string) error {
	if change == "" || change == "." || change == ".." || change == "_shared" || strings.ContainsAny(change, `/\:<>"|?*`) || strings.TrimSpace(change) != change || strings.HasSuffix(change, ".") || !utf8.ValidString(change) {
		return fmt.Errorf("invalid change ID %q", change)
	}
	for _, r := range change {
		if unicode.IsControl(r) {
			return fmt.Errorf("invalid change ID %q", change)
		}
	}
	base := strings.ToUpper(strings.SplitN(change, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
		return fmt.Errorf("reserved change ID %q", change)
	}
	return nil
}
