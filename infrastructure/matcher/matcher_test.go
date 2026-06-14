package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatcher_Match(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		// exact match (no wildcards) stays backward compatible
		{name: "exact match", pattern: "home", path: "home", want: true},
		{name: "exact mismatch", pattern: "home", path: "about", want: false},
		{name: "exact multi-segment match", pattern: "articles/uuid-1", path: "articles/uuid-1", want: true},
		{name: "exact multi-segment mismatch", pattern: "articles/uuid-1", path: "articles/uuid-2", want: false},

		// "*" matches a single segment
		{name: "single wildcard matches a segment", pattern: "/*/home", path: "/en/home", want: true},
		{name: "single wildcard matches another segment", pattern: "/*/home", path: "/fr/home", want: true},
		{name: "single wildcard does not span segments", pattern: "/*/home", path: "/en/some/home", want: false},
		{name: "single wildcard requires a segment", pattern: "/*/home", path: "/home", want: false},
		{name: "trailing single wildcard", pattern: "articles/*", path: "articles/uuid-1", want: true},
		{name: "trailing single wildcard needs a segment", pattern: "articles/*", path: "articles", want: false},

		// "**" matches zero or more segments
		{name: "double wildcard single segment", pattern: "/**/articles", path: "/en/articles", want: true},
		{name: "double wildcard many segments", pattern: "/**/articles", path: "/en/some/other/path/articles", want: true},
		{name: "double wildcard zero segments", pattern: "/**/articles", path: "/articles", want: true},
		{name: "double wildcard mismatch suffix", pattern: "/**/articles", path: "/en/comments", want: false},
		{name: "trailing double wildcard matches rest", pattern: "articles/**", path: "articles/a/b/c", want: true},
		{name: "trailing double wildcard matches nothing", pattern: "articles/**", path: "articles", want: true},

		// "?" matches a single character within a segment
		{name: "question mark single char", pattern: "/home?", path: "/home1", want: true},
		{name: "question mark another char", pattern: "/home?", path: "/home2", want: true},
		{name: "question mark requires a char", pattern: "/home?", path: "/home", want: false},
		{name: "question mark single char only", pattern: "/home?", path: "/home12", want: false},

		// combinations
		{name: "single and double wildcards", pattern: "/*/**/show", path: "/en/a/b/show", want: true},
		{name: "double then exact", pattern: "**/edit", path: "en/articles/edit", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, New().Match(tt.pattern, tt.path))
		})
	}
}
