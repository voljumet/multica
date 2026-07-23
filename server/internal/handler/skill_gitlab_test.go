package handler

import (
	"testing"
)

func TestGitlabConfiguredHost(t *testing.T) {
	t.Setenv("GITLAB_URL", "https://gitlab.company.com")
	if got := gitlabConfiguredHost(); got != "gitlab.company.com" {
		t.Fatalf("got %q, want %q", got, "gitlab.company.com")
	}

	t.Setenv("GITLAB_URL", "")
	if got := gitlabConfiguredHost(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestParseGitLabURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantNS  string
		wantRef string
		wantDir string
		wantErr bool
	}{
		{
			name:    "root no path",
			url:     "https://gitlab.company.com/group/repo",
			wantNS:  "group/repo",
			wantRef: "",
			wantDir: "",
		},
		{
			name:    "tree with dash",
			url:     "https://gitlab.company.com/group/repo/-/tree/main/my-skill",
			wantNS:  "group/repo",
			wantRef: "main",
			wantDir: "my-skill",
		},
		{
			name:    "tree without dash",
			url:     "https://gitlab.company.com/group/repo/tree/main/my-skill",
			wantNS:  "group/repo",
			wantRef: "main",
			wantDir: "my-skill",
		},
		{
			name:    "nested subgroup",
			url:     "https://gitlab.company.com/group/sub/repo/-/tree/main/skill-a",
			wantNS:  "group/sub/repo",
			wantRef: "main",
			wantDir: "skill-a",
		},
		{
			name:    "nested skill dir",
			url:     "https://gitlab.company.com/group/repo/-/tree/main/skills/foo",
			wantNS:  "group/repo",
			wantRef: "main",
			wantDir: "skills/foo",
		},
		{
			name:    "root with no segments after host",
			url:     "https://gitlab.company.com/",
			wantErr: true,
		},
		{
			name:    "only owner no repo",
			url:     "https://gitlab.company.com/group",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := parseGitLabURL(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if spec.namespace != tc.wantNS {
				t.Errorf("namespace = %q, want %q", spec.namespace, tc.wantNS)
			}
			if spec.ref != tc.wantRef {
				t.Errorf("ref = %q, want %q", spec.ref, tc.wantRef)
			}
			if spec.skillDir != tc.wantDir {
				t.Errorf("skillDir = %q, want %q", spec.skillDir, tc.wantDir)
			}
		})
	}
}

func TestDetectImportSource_GitLab(t *testing.T) {
	t.Setenv("GITLAB_URL", "https://gitlab.company.com")

	source, normalized, err := detectImportSource("https://gitlab.company.com/group/repo/-/tree/main/skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != sourceGitLab {
		t.Fatalf("source = %v, want sourceGitLab", source)
	}
	if normalized == "" {
		t.Fatal("normalized URL empty")
	}
}

func TestDetectImportSource_GitLabNotConfigured(t *testing.T) {
	t.Setenv("GITLAB_URL", "")

	_, _, err := detectImportSource("https://gitlab.company.com/group/repo")
	if err == nil {
		t.Fatal("expected error when GITLAB_URL not set")
	}
}
