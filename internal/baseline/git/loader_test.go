package git

import (
	"regexp"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestAuthorExcluded(t *testing.T) {
	botPattern := regexp.MustCompile(`\[bot\]`)
	dependabotEmail := regexp.MustCompile(`(?i)dependabot`)

	cases := []struct {
		name     string
		sig      object.Signature
		patterns []*regexp.Regexp
		want     bool
	}{
		{
			name:     "no patterns means no exclusion",
			sig:      object.Signature{Name: "dependabot[bot]", Email: "x@y.z"},
			patterns: nil,
			want:     false,
		},
		{
			name:     "matches name with bot suffix",
			sig:      object.Signature{Name: "dependabot[bot]", Email: "49699333+dependabot[bot]@users.noreply.github.com"},
			patterns: []*regexp.Regexp{botPattern},
			want:     true,
		},
		{
			name:     "matches email but not name",
			sig:      object.Signature{Name: "renovatebot", Email: "29139614+renovate[bot]@users.noreply.github.com"},
			patterns: []*regexp.Regexp{botPattern},
			want:     true,
		},
		{
			name:     "skips human author",
			sig:      object.Signature{Name: "Aguinelo Koczkodai", Email: "aguinelo@gmail.com"},
			patterns: []*regexp.Regexp{botPattern, dependabotEmail},
			want:     false,
		},
		{
			name:     "ignores nil pattern entries",
			sig:      object.Signature{Name: "dependabot[bot]"},
			patterns: []*regexp.Regexp{nil, botPattern},
			want:     true,
		},
		{
			name:     "empty signature never matches",
			sig:      object.Signature{},
			patterns: []*regexp.Regexp{botPattern},
			want:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := authorExcluded(tc.sig, tc.patterns)
			if got != tc.want {
				t.Errorf("authorExcluded(%+v) = %v, want %v", tc.sig, got, tc.want)
			}
		})
	}
}
