package profiler

import (
	"fmt"
	"regexp"

	"github.com/google/pprof/profile"
)

// redactedPlaceholder replaces every match of a sensitive pattern.
const redactedPlaceholder = "[REDACTED]"

// builtinRedactPatterns cover the sensitive material called out by the blog
// post: credentials in key=value shape and e-mail addresses. IPv4 addresses
// are opt-in via Config.RedactIPs.
var builtinRedactPatterns = []string{
	`(?i)(?:password|passwd|pwd|secret|token|api[_-]?key|authorization|bearer|credential)[\s=:]+\S+`,
	`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`,
}

// sensitiveKeyPattern flags pprof label keys whose values are compromised by
// definition (a label named "...token" carries a token, whatever its value).
const sensitiveKeyPattern = `(?i)(?:password|passwd|pwd|secret|token|api[_-]?key|authorization|bearer|credential)`

const ipv4Pattern = `\b(?:\d{1,3}\.){3}\d{1,3}\b`

// sanitizer redacts sensitive information from parsed pprof profiles before
// they are converted and exported. Profiles carry free-form strings in
// sample labels, comments, and symbol/file names — all of which may leak
// secrets baked into code or request-scoped label values.
type sanitizer struct {
	patterns    []*regexp.Regexp
	keyPatterns []*regexp.Regexp
}

func newSanitizer(extraPatterns []string, redactIPs bool) (*sanitizer, error) {
	raw := make([]string, 0, len(builtinRedactPatterns)+len(extraPatterns)+1)
	raw = append(raw, builtinRedactPatterns...)
	raw = append(raw, extraPatterns...)
	if redactIPs {
		raw = append(raw, ipv4Pattern)
	}

	patterns := make([]*regexp.Regexp, 0, len(raw))
	for _, p := range raw {
		compiled, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("profiler: invalid redact pattern %q: %w", p, err)
		}
		patterns = append(patterns, compiled)
	}

	// custom patterns also apply to label keys
	keyPatterns := make([]*regexp.Regexp, 0, len(extraPatterns)+1)
	keyPatterns = append(keyPatterns, regexp.MustCompile(sensitiveKeyPattern))
	keyPatterns = append(keyPatterns, patterns[len(builtinRedactPatterns):len(builtinRedactPatterns)+len(extraPatterns)]...)

	return &sanitizer{patterns: patterns, keyPatterns: keyPatterns}, nil
}

// sanitize redacts p in place.
func (s *sanitizer) sanitize(p *profile.Profile) {
	for i, c := range p.Comments {
		p.Comments[i] = s.redact(c)
	}

	for _, fn := range p.Function {
		fn.Name = s.redact(fn.Name)
		fn.SystemName = s.redact(fn.SystemName)
		fn.Filename = s.redact(fn.Filename)
	}

	for _, m := range p.Mapping {
		m.File = s.redact(m.File)
	}

	for _, sample := range p.Sample {
		s.redactLabels(sample.Label)
		s.redactNumLabelKeys(sample)
	}
}

// redactLabels rewrites label values and drops labels whose key itself is
// sensitive; the value of such a label is compromised no matter its content.
func (s *sanitizer) redactLabels(labels map[string][]string) {
	for key, values := range labels {
		if s.matches(key) {
			delete(labels, key)
			continue
		}

		for i, v := range values {
			values[i] = s.redact(v)
		}
	}
}

func (s *sanitizer) redactNumLabelKeys(sample *profile.Sample) {
	for key := range sample.NumLabel {
		if s.matches(key) {
			delete(sample.NumLabel, key)
			delete(sample.NumUnit, key)
		}
	}
}

func (s *sanitizer) redact(v string) string {
	for _, p := range s.patterns {
		v = p.ReplaceAllString(v, redactedPlaceholder)
	}

	return v
}

func (s *sanitizer) matches(key string) bool {
	for _, p := range s.keyPatterns {
		if p.MatchString(key) {
			return true
		}
	}

	return false
}
