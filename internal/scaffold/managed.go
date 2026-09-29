package scaffold

import (
	"bytes"
	"io/fs"
	"path"
	"sort"

	"github.com/mulix-dev/mulix-coding/assets"
)

// managedFile is one file mulix installs into a project and keeps up to
// date: its root-relative, slash-separated destination and its bundled
// content.
type managedFile struct {
	Rel  string
	Data []byte
}

// Mode returns the permission bits to install f with: executable for
// scripts (a "#!" first line), since several embedded skills run their
// helpers directly rather than through `bash <script>`.
func (f managedFile) Mode() fs.FileMode {
	if bytes.HasPrefix(f.Data, []byte("#!")) {
		return 0o755
	}
	return 0o644
}

// SuperpowersDir is where the embedded superpowers skills' license and
// version stamp are installed. The skills themselves go to .claude/skills/
// like mulix's own.
const SuperpowersDir = ".mulix/superpowers"

// managedFiles lists every file init installs and update refreshes, in a
// stable order:
//
//   - mulix's phase skills: assets/skills/<name>/** → .claude/skills/<name>/**
//   - every embedded superpowers skill, whole directory (SKILL.md plus its
//     prompts, references, and scripts): assets/superpowers/skills/<name>/**
//     → .claude/skills/<name>/**
//   - the superpowers license and version: → .mulix/superpowers/
//   - templates: assets/templates/* → .mulix/templates/*
//
// The constitution is deliberately not here: it's seeded once by init and
// is the user's document from then on.
func managedFiles() ([]managedFile, error) {
	var out []managedFile
	add := func(fsys fs.FS, srcRoot, destRoot string) error {
		return fs.WalkDir(fsys, srcRoot, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			rel := path.Join(destRoot, p[len(srcRoot)+1:])
			out = append(out, managedFile{Rel: rel, Data: data})
			return nil
		})
	}
	if err := add(assets.Skills, "skills", ".claude/skills"); err != nil {
		return nil, err
	}
	if err := add(assets.Superpowers, "superpowers/skills", ".claude/skills"); err != nil {
		return nil, err
	}
	for _, name := range []string{"LICENSE", "VERSION"} {
		data, err := fs.ReadFile(assets.Superpowers, "superpowers/"+name)
		if err != nil {
			return nil, err
		}
		out = append(out, managedFile{Rel: path.Join(SuperpowersDir, name), Data: data})
	}
	if err := add(assets.Templates, "templates", ".mulix/templates"); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

// obsoleteFiles are files older mulix versions installed that this one
// no longer ships. update removes each one that is still byte-identical
// to its install baseline, and reports the rest for manual cleanup.
var obsoleteFiles = []string{
	".claude/skills/mulix-using-mulix/SKILL.md", // renamed to using-mulix
	".claude/skills/mulix-plan/SKILL.md",        // plan phase folded into design (brainstorming)
	".claude/skills/mulix-analyze/SKILL.md",     // analyze phase folded into design/tasks
	".mulix/templates/plan-template.md",
	".mulix/templates/analyze-template.md",
}
