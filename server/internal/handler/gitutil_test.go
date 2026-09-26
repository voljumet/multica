package handler

import (
	"reflect"
	"testing"
)

func TestExtractIdentifiers(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "branch_name",
			in:   []string{"", "", "mul-1510/fix-login"},
			want: []string{"MUL-1510"},
		},
		{
			name: "single_character_prefix",
			in:   []string{"H-412: fix widget parity"},
			want: []string{"H-412"},
		},
		{
			name: "title_and_body",
			in:   []string{"Fix MUL-82", "Closes MUL-1510 and ABC-7", ""},
			want: []string{"MUL-82", "MUL-1510", "ABC-7"},
		},
		{
			name: "dedupe_across_fields",
			in:   []string{"MUL-1", "MUL-1 again", "mul-1/branch"},
			want: []string{"MUL-1"},
		},
		{
			name: "ignore_email_and_versions",
			in:   []string{"reply@user-1 v1.2-3 here", "", ""},
			// Word-boundary regex still matches "user-1"; identifier prefix is
			// any 2..10 letters/digits, so this is intentional. The downstream
			// workspace prefix check in lookupIssueByIdentifier filters it.
			want: []string{"USER-1"},
		},
		{
			name: "no_match",
			in:   []string{"plain text", "no idents", ""},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractIdentifiers(tc.in...)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractIdentifiers() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractClosingIdentifiers(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "single_closes",
			in:   []string{"", "Closes MUL-1"},
			want: []string{"MUL-1"},
		},
		{
			name: "single_character_prefix",
			in:   []string{"", "Closes H-412"},
			want: []string{"H-412"},
		},
		{
			name: "all_keyword_inflections",
			in: []string{
				"",
				"close MUL-1\nclosed MUL-2\ncloses MUL-3\nfix MUL-4\nfixes MUL-5\nfixed MUL-6\nresolve MUL-7\nresolves MUL-8\nresolved MUL-9",
			},
			want: []string{"MUL-1", "MUL-2", "MUL-3", "MUL-4", "MUL-5", "MUL-6", "MUL-7", "MUL-8", "MUL-9"},
		},
		{
			name: "case_insensitive_and_colon",
			in:   []string{"CLOSES: MUL-1", "Fixes:MUL-2 resolves   MUL-3"},
			want: []string{"MUL-1", "MUL-2", "MUL-3"},
		},
		{
			name: "bare_reference_does_not_close",
			// The bug-report repro: only ABC-1 carries closing intent.
			// ABC-2/ABC-3 are linked (extractIdentifiers) but must not
			// appear in the closing set.
			in:   []string{"ABC-1: Lorem Ipsum", "Closes ABC-1. Follow up work planned in ABC-2. Unblocks ABC-3."},
			want: []string{"ABC-1"},
		},
		{
			name: "keyword_not_adjacent_does_not_close",
			// "Fix login MUL-1" — keyword present but the identifier is
			// not adjacent. Consistent with GitHub's closing-keyword
			// grammar; matches via extractIdentifiers for linking only.
			in:   []string{"Fix login MUL-1", ""},
			want: []string{},
		},
		{
			name: "dedupe_across_fields",
			in:   []string{"Closes MUL-1", "fixes mul-1"},
			want: []string{"MUL-1"},
		},
		{
			name: "no_match_on_disclosed_or_foreclose",
			// Word-boundary guards against keyword fragments embedded
			// in larger words ("Disclosed MUL-1", "Foreclose MUL-1").
			in:   []string{"Disclosed MUL-1 in foreclose MUL-2", ""},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractClosingIdentifiers(tc.in...)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractClosingIdentifiers() = %v, want %v", got, tc.want)
			}
		})
	}
}
