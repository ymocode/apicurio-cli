// Package labels parses user-supplied key/value labels from CLI flags into the
// map[string]string form expected by the registry version metadata.
package labels

import (
	"fmt"
	"strings"
)

// Parse converts raw flag values into a label map.
//
// Each raw value may hold a single label or several labels separated by commas,
// so repeated flags and comma-separated values are equivalent:
//
//	["a=1", "b=2"]   and   ["a=1,b=2"]   both yield {"a":"1", "b":"2"}
//
// Every label uses the form key=value. The value is taken from the first "="
// onward, so values may themselves contain "=". Keys and values are trimmed of
// surrounding whitespace. An empty value (key=) is allowed; an empty key, or a
// segment without "=", is an error. Empty segments (e.g. from a trailing comma)
// are ignored.
//
// Parse returns nil when raw contains no labels, allowing callers to treat the
// absence of labels as a no-op.
func Parse(raw []string) (map[string]string, error) {
	result := make(map[string]string)

	for _, value := range raw {
		for segment := range strings.SplitSeq(value, ",") {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				continue
			}

			key, val, found := strings.Cut(segment, "=")
			if !found {
				return nil, fmt.Errorf("invalid label %q: expected key=value", segment)
			}

			key = strings.TrimSpace(key)
			if key == "" {
				return nil, fmt.Errorf("invalid label %q: key must not be empty", segment)
			}

			result[key] = strings.TrimSpace(val)
		}
	}

	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}
