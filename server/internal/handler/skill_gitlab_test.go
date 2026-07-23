package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestFetchFromGitLab(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Go's HTTP server decodes %2F in r.URL.Path, so namespace "group/repo"
		// appears as /api/v4/projects/group/repo (not group%2Frepo).
		switch r.URL.Path {
		case "/api/v4/projects/group/repo":
			_ = json.NewEncoder(w).Encode(map[string]any{"default_branch": "main"})
		case "/api/v4/projects/group/repo/repository/files/SKILL.md/raw":
			_, _ = w.Write([]byte("---\nname: My Skill\ndescription: A test skill\n---\n# Content"))
		case "/api/v4/projects/group/repo/repository/tree":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"type": "blob", "path": "SKILL.md", "name": "SKILL.md"},
				{"type": "blob", "path": "notes.md", "name": "notes.md"},
			})
		case "/api/v4/projects/group/repo/repository/files/notes.md/raw":
			_, _ = w.Write([]byte("extra notes"))
		default:
			t.Logf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("GITLAB_URL", srv.URL)

	skill, err := fetchFromGitLab(srv.Client(), "test-token", srv.URL+"/group/repo")
	if err != nil {
		t.Fatalf("fetchFromGitLab: %v", err)
	}
	if skill.name != "My Skill" {
		t.Errorf("name = %q, want %q", skill.name, "My Skill")
	}
	if skill.description != "A test skill" {
		t.Errorf("description = %q", skill.description)
	}
	if skill.origin["type"] != "gitlab" {
		t.Errorf("origin.type = %v, want %q", skill.origin["type"], "gitlab")
	}
	if skill.origin["source_url"] != srv.URL+"/group/repo" {
		t.Errorf("origin.source_url = %v", skill.origin["source_url"])
	}
	foundNotes := false
	for _, f := range skill.files {
		if f.path == "notes.md" && f.content == "extra notes" {
			foundNotes = true
		}
	}
	if !foundNotes {
		t.Errorf("expected notes.md in files, got %v", skill.files)
	}
}

func TestFetchFromGitLab_SkillDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// file path "skills/foo/SKILL.md" encoded as skills%2Ffoo%2FSKILL.md in the
		// URL but decoded by Go HTTP server to skills/foo/SKILL.md in r.URL.Path.
		switch r.URL.Path {
		case "/api/v4/projects/group/repo/repository/files/skills/foo/SKILL.md/raw":
			_, _ = w.Write([]byte("---\nname: Foo Skill\n---\n# Foo"))
		case "/api/v4/projects/group/repo/repository/tree":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"type": "blob", "path": "skills/foo/SKILL.md", "name": "SKILL.md"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("GITLAB_URL", srv.URL)

	skill, err := fetchFromGitLab(srv.Client(), "tok", srv.URL+"/group/repo/-/tree/main/skills/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.name != "Foo Skill" {
		t.Errorf("name = %q, want %q", skill.name, "Foo Skill")
	}
}
