package cliutil

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mulix-dev/mulix-coding/assets"
	"github.com/mulix-dev/mulix-coding/internal/preset"
	"github.com/mulix-dev/mulix-coding/internal/state"
)

// isLocalDir reports whether source names an existing directory, used by
// `preset add` to distinguish a local-path source from a URL or catalog id.
func isLocalDir(source string) bool {
	info, err := os.Stat(source)
	return err == nil && info.IsDir()
}

// removePresetFiles deletes a preset's installed directory. Called only
// after preset.Remove has already dropped the registry entry, so a failed
// delete here leaves an orphaned-but-harmless directory rather than a
// registry entry pointing at nothing.
func removePresetFiles(root, id string) error {
	dir := preset.InstallDir(root, id)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("preset: deleting %s: %w", dir, err)
	}
	fmt.Printf("deleted %s\n", dir)
	return nil
}

func newPresetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preset",
		Short: "Manage template-override presets (project overrides > presets by priority > core bundled)",
	}
	cmd.AddCommand(
		newPresetListCmd(),
		newPresetAddCmd(),
		newPresetRemoveCmd(),
		newPresetEnableCmd(),
		newPresetDisableCmd(),
		newPresetSetPriorityCmd(),
		newPresetInfoCmd(),
		newPresetResolveCmd(),
		newPresetSearchCmd(),
		newPresetCatalogCmd(),
	)
	return cmd
}

func newPresetListCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed presets in priority order (highest precedence first)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			reg, err := preset.LoadRegistry(root)
			if err != nil {
				return err
			}
			entries := preset.ListByPriority(reg, all)
			if len(entries) == 0 {
				fmt.Println("no presets installed (use `mulix preset add <source>`)")
				return nil
			}
			for _, e := range entries {
				status := "enabled"
				if !e.Entry.Enabled {
					status = "disabled"
				}
				fmt.Printf("%-20s priority=%-4d %-8s version=%s source=%s\n", e.ID, e.Entry.Priority, status, e.Entry.Version, e.Entry.Source)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include disabled presets")
	return cmd
}

func newPresetAddCmd() *cobra.Command {
	var sha256Hex string
	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Install a preset from a local directory, an HTTPS zip URL, or a bundled/configured catalog id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			res, err := installPresetFromSource(root, args[0], sha256Hex)
			if err != nil {
				return err
			}
			fmt.Printf("installed %q version %s at %s\n", res.ID, res.Version, res.Dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&sha256Hex, "sha256", "", "expected sha256 of the downloaded archive (only used for URL/catalog sources)")
	return cmd
}

// installPresetFromSource dispatches add's <source> argument: a local
// directory (if it exists on disk), an https(s):// URL, or otherwise a
// catalog id looked up across the bundled catalog and any configured via
// `mulix preset catalog add`.
func installPresetFromSource(root, source, sha256Hex string) (preset.InstallResult, error) {
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		return preset.InstallFromURL(root, source, sha256Hex)
	}
	if isLocalDir(source) {
		return preset.InstallFromDir(root, source)
	}

	entry, err := lookupCatalogEntry(root, source)
	if err != nil {
		return preset.InstallResult{}, err
	}
	if entry.DownloadURL == "" {
		return preset.InstallResult{}, fmt.Errorf("preset: catalog entry %q has no download_url (discovery only; install via a direct URL instead)", source)
	}
	hash := sha256Hex
	if hash == "" {
		hash = entry.SHA256
	}
	return preset.InstallFromURL(root, entry.DownloadURL, hash)
}

func newPresetRemoveCmd() *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Unregister an installed preset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			if err := preset.Remove(root, args[0]); err != nil {
				return err
			}
			fmt.Printf("removed %q from the registry\n", args[0])
			if !purge {
				fmt.Printf("its files remain at %s; re-run with --purge to delete them\n", preset.InstallDir(root, args[0]))
				return nil
			}
			return removePresetFiles(root, args[0])
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also delete the preset's files from .mulix/presets/<id>/")
	return cmd
}

func newPresetEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "Re-enable a disabled preset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			if err := preset.SetEnabled(root, args[0], true); err != nil {
				return err
			}
			fmt.Printf("enabled %q\n", args[0])
			return nil
		},
	}
}

func newPresetDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: "Disable a preset without uninstalling it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			if err := preset.SetEnabled(root, args[0], false); err != nil {
				return err
			}
			fmt.Printf("disabled %q\n", args[0])
			return nil
		},
	}
}

func newPresetSetPriorityCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-priority <id> <n>",
		Short: "Set a preset's priority (lower number = checked first, wins ties over higher numbers)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			n, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("priority must be an integer, got %q", args[1])
			}
			if err := preset.SetPriority(root, args[0], n); err != nil {
				return err
			}
			fmt.Printf("set %q priority to %d\n", args[0], n)
			return nil
		},
	}
}

func newPresetInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <id>",
		Short: "Show an installed preset's registry entry and declared templates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			reg, err := preset.LoadRegistry(root)
			if err != nil {
				return err
			}
			entry, ok := reg.Presets[args[0]]
			if !ok {
				return fmt.Errorf("preset: %q is not installed", args[0])
			}
			fmt.Printf("id:            %s\n", args[0])
			fmt.Printf("version:       %s\n", entry.Version)
			fmt.Printf("source:        %s\n", entry.Source)
			fmt.Printf("priority:      %d\n", entry.Priority)
			fmt.Printf("enabled:       %v\n", entry.Enabled)
			fmt.Printf("manifest_hash: %s\n", entry.ManifestHash)
			fmt.Printf("installed_at:  %s\n", entry.InstalledAt.Format("2006-01-02T15:04:05Z"))

			manifestPath := preset.InstallDir(root, args[0]) + "/preset.yml"
			m, err := preset.LoadManifest(manifestPath)
			if err != nil {
				fmt.Printf("(could not load manifest: %v)\n", err)
				return nil
			}
			fmt.Println("templates:")
			for _, t := range m.Provides.Templates {
				fmt.Printf("  - %-20s strategy=%-8s file=%s\n", t.Name, t.EffectiveStrategy(), t.File)
			}
			return nil
		},
	}
}

func newPresetResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <template-name>",
		Short: "Print the composed content mulix would use for a template name (project overrides > presets > core)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			content, err := preset.ResolveTemplateContent(root, args[0], assets.Templates)
			if err != nil {
				return err
			}
			fmt.Print(content)
			return nil
		},
	}
}

func newPresetSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search the bundled catalog and any configured catalogs by id, name, description, or tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			cats, err := allCatalogs(root)
			if err != nil {
				return err
			}
			found := 0
			for _, cat := range cats {
				for _, e := range cat.Search(args[0]) {
					fmt.Printf("[%s] %-20s %s\n", cat.Name, e.ID, e.Description)
					found++
				}
			}
			if found == 0 {
				fmt.Println("no matching presets found")
			}
			return nil
		},
	}
}

func newPresetCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Manage the set of catalogs `preset search`/`preset add` look across",
	}
	cmd.AddCommand(newPresetCatalogListCmd(), newPresetCatalogAddCmd(), newPresetCatalogRemoveCmd())
	return cmd
}

func newPresetCatalogListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active catalogs (the bundled 'default' catalog is always active)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			fmt.Println("default   (bundled)")
			refs, err := preset.LoadCatalogRefs(root)
			if err != nil {
				return err
			}
			for _, r := range refs {
				fmt.Printf("%-10s %s\n", r.Name, r.URL)
			}
			return nil
		},
	}
}

func newPresetCatalogAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add a catalog by name and URL to a JSON catalog file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			if err := preset.AddCatalogRef(root, args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("added catalog %q -> %s\n", args[0], args[1])
			return nil
		},
	}
}

func newPresetCatalogRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a configured catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			if err := preset.RemoveCatalogRef(root, args[0]); err != nil {
				return err
			}
			fmt.Printf("removed catalog %q\n", args[0])
			return nil
		},
	}
}

// allCatalogs returns the bundled catalog plus every catalog configured
// for root, sorted by name (bundled "default" always first) so search
// output order is stable.
func allCatalogs(root string) ([]preset.Catalog, error) {
	bundled, err := preset.LoadBundledCatalog(assets.Presets)
	if err != nil {
		return nil, err
	}
	cats := []preset.Catalog{bundled}

	refs, err := preset.LoadCatalogRefs(root)
	if err != nil {
		return nil, err
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	for _, ref := range refs {
		cat, err := preset.FetchCatalog(ref)
		if err != nil {
			return nil, fmt.Errorf("fetching catalog %q: %w", ref.Name, err)
		}
		cats = append(cats, cat)
	}
	return cats, nil
}

// lookupCatalogEntry finds source as a preset id across the bundled
// catalog and every configured catalog, preferring the first match in
// that order (bundled "default" takes precedence, matching spec-kit's
// catalog-stack priority convention).
func lookupCatalogEntry(root, source string) (preset.CatalogEntry, error) {
	cats, err := allCatalogs(root)
	if err != nil {
		return preset.CatalogEntry{}, err
	}
	for _, cat := range cats {
		if e, ok := cat.Entries[source]; ok {
			return e, nil
		}
	}
	return preset.CatalogEntry{}, fmt.Errorf("preset: %q is not a local directory, URL, or known catalog id", source)
}
