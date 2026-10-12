package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gammons/slk/internal/config"
)

// workspaceSavers are the savers that write into a team's
// [workspaces.*] block, each saving a value for team T01ABCDEF that
// saved can find in the loaded block.
var workspaceSavers = []struct {
	name  string
	save  func(path string) error
	saved func(config.Workspace) bool
}{
	{
		"theme",
		func(path string) error { return saveWorkspaceTheme(path, "T01ABCDEF", "Acme Corp", "dracula") },
		func(ws config.Workspace) bool { return ws.Theme == "dracula" },
	},
	{
		"sidebar width",
		func(path string) error { return saveWorkspaceWidth(path, "T01ABCDEF", "Acme Corp", 42) },
		func(ws config.Workspace) bool { return ws.SidebarWidth == 42 },
	},
	{
		"version_ts",
		func(path string) error { return saveWorkspaceVersionTS(path, "T01ABCDEF", "Acme Corp", "1791734257") },
		func(ws config.Workspace) bool { return ws.VersionTS == "1791734257" },
	},
}

// The first launch onboards, which writes a slug-keyed block for the
// team after run() has loaded the config. The savers used to resolve
// the block from that earlier load, miss it, and append a second
// [workspaces.<teamID>] block, so the next launch failed with
// "workspaces "acme" and "T01ABCDEF" both reference team_id".
func TestWorkspaceSaversWriteIntoSlugBlockOnDisk(t *testing.T) {
	for _, s := range workspaceSavers {
		t.Run(s.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			onboarded := "# Acme Corp\n[workspaces.acme]\nteam_id = \"T01ABCDEF\"\n"
			if err := os.WriteFile(path, []byte(onboarded), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := s.save(path); err != nil {
				t.Fatalf("save: %v", err)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("config.Load after save: %v\n%s", err, mustRead(t, path))
			}
			if !s.saved(cfg.Workspaces["acme"]) {
				t.Errorf("value not saved into [workspaces.acme]:\n%s", mustRead(t, path))
			}
		})
	}
}

// A saver must not write a config it cannot update in place: one
// config.Load already rejects (appending cannot repair it), or one
// whose block for the team has a header the textual rewrite cannot
// find (appending would duplicate the team_id).
func TestWorkspaceSaversLeaveUnupdatableConfigUntouched(t *testing.T) {
	configs := []struct {
		name string
		toml string
	}{
		{
			"duplicate team_id",
			"[workspaces.acme]\nteam_id = \"T01ABCDEF\"\n\n[workspaces.T01ABCDEF]\nversion_ts = \"1785403654\"\n",
		},
		{
			"header followed by a comment",
			"[workspaces.acme] # work\nteam_id = \"T01ABCDEF\"\n",
		},
	}
	for _, c := range configs {
		for _, s := range workspaceSavers {
			t.Run(c.name+"/"+s.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(path, []byte(c.toml), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := s.save(path); err == nil {
					t.Error("save succeeded; want an error")
				}
				if got := mustRead(t, path); !bytes.Equal(got, []byte(c.toml)) {
					t.Errorf("config changed:\n%s", got)
				}
			})
		}
	}
}
