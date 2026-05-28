package main

import (
	"bytes"
	"errors"
	"fmt"
	iofs "io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
)

const (
	initVendorDir    = "packs/vendor"
	maxVendoredPacks = 50
)

type localVendorWork struct {
	srcPackDir   string
	cityTomlPath string
}

type vendoredPack struct {
	cityRelPath string
	vendorDst   string
}

type escapingImport struct {
	absSourcePath  string
	originalSource string
}

func vendorExternalLocalPackDeps(fs fsys.FS, srcDir, cityPath string) ([]string, error) {
	absSrcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return nil, fmt.Errorf("vendoring local deps: resolving source %q: %w", srcDir, err)
	}
	absCityPath, err := filepath.Abs(cityPath)
	if err != nil {
		return nil, fmt.Errorf("vendoring local deps: resolving city %q: %w", cityPath, err)
	}
	queue, err := seedVendorQueue(fs, absCityPath, absSrcDir)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]vendoredPack)
	return processVendorWork(fs, queue, seen, absSrcDir, absCityPath)
}

func seedVendorQueue(fs fsys.FS, cityPath, srcDir string) ([]localVendorWork, error) {
	var queue []localVendorWork
	var walk func(dir, relDir string) error
	walk = func(dir, relDir string) error {
		entries, err := fs.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("vendoring local deps: walking %q: %w", dir, err)
		}
		for _, entry := range entries {
			relPath := entry.Name()
			if relDir != "" {
				relPath = filepath.Join(relDir, entry.Name())
			}
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				if initFromSkip(relPath, true) {
					continue
				}
				if err := walk(path, relPath); err != nil {
					return err
				}
				continue
			}
			if entry.Name() != "city.toml" && entry.Name() != "pack.toml" {
				continue
			}
			srcRelDir := relDir
			queue = append(queue, localVendorWork{
				srcPackDir:   filepath.Join(srcDir, srcRelDir),
				cityTomlPath: path,
			})
		}
		return nil
	}
	if err := walk(cityPath, ""); err != nil {
		return nil, err
	}
	return queue, nil
}

func processVendorWork(fs fsys.FS, queue []localVendorWork, seen map[string]vendoredPack, srcDir, cityPath string) ([]string, error) {
	var vendored []string
	for i := 0; i < len(queue); i++ {
		work := queue[i]
		imports, err := extractEscapingImports(fs, work.cityTomlPath, work.srcPackDir, srcDir)
		if err != nil {
			return nil, err
		}
		rewrites := make(map[string]string)
		for _, imp := range imports {
			if existing, ok := seen[imp.absSourcePath]; ok {
				rewrites[imp.originalSource] = existing.cityRelPath
				continue
			}
			if len(seen) >= maxVendoredPacks {
				return nil, fmt.Errorf("vendoring local deps: dependency graph exceeds %d packs", maxVendoredPacks)
			}
			if _, err := fs.Stat(imp.absSourcePath); err != nil {
				if errors.Is(err, iofs.ErrNotExist) {
					return nil, fmt.Errorf("vendoring local deps: pack %q declared in %q not found: %w", imp.absSourcePath, work.cityTomlPath, err)
				}
				return nil, fmt.Errorf("vendoring local deps: stating pack %q declared in %q: %w", imp.absSourcePath, work.cityTomlPath, err)
			}
			name := chooseVendorName(imp.absSourcePath, seen, cityPath)
			cityRelPath := "//" + filepath.ToSlash(filepath.Join(initVendorDir, name))
			dst := filepath.Join(cityPath, initVendorDir, name)
			if err := copyDirViaFS(fs, imp.absSourcePath, dst); err != nil {
				return nil, fmt.Errorf("vendoring local deps: copying %q to %q: %w", imp.absSourcePath, dst, err)
			}
			seen[imp.absSourcePath] = vendoredPack{cityRelPath: cityRelPath, vendorDst: dst}
			vendored = append(vendored, name)
			rewrites[imp.originalSource] = cityRelPath
			queue = append(queue, localVendorWork{
				srcPackDir:   imp.absSourcePath,
				cityTomlPath: filepath.Join(dst, "pack.toml"),
			})
		}
		if len(rewrites) == 0 {
			continue
		}
		if err := rewriteImportSources(fs, work.cityTomlPath, rewrites); err != nil {
			return nil, err
		}
	}
	sort.Strings(vendored)
	return vendored, nil
}

func extractEscapingImports(fs fsys.FS, cityTomlPath, srcPackDir, srcDir string) ([]escapingImport, error) {
	data, err := fs.ReadFile(cityTomlPath)
	if err != nil {
		return nil, fmt.Errorf("vendoring local deps: reading %q: %w", cityTomlPath, err)
	}
	var sources []string
	switch filepath.Base(cityTomlPath) {
	case "city.toml":
		cfg, err := config.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("vendoring local deps: parsing %q: %w", cityTomlPath, err)
		}
		sources = appendImportSources(sources, cfg.Imports)
		sources = append(sources, cfg.Workspace.LegacyIncludes()...)
		sources = appendImportSources(sources, cfg.Defaults.Rig.Imports)
		for _, rig := range cfg.Rigs {
			sources = appendImportSources(sources, rig.Imports)
			sources = append(sources, rig.Includes...)
		}
	case "pack.toml":
		var cfg config.PackConfig
		if _, err := toml.Decode(string(data), &cfg); err != nil {
			return nil, fmt.Errorf("vendoring local deps: parsing %q: %w", cityTomlPath, err)
		}
		sources = appendImportSources(sources, cfg.Imports)
		sources = append(sources, cfg.Pack.Includes...)
	default:
		return nil, nil
	}

	var imports []escapingImport
	for _, source := range sources {
		imp, ok, err := escapingImportForSource(source, srcPackDir, srcDir)
		if err != nil {
			return nil, fmt.Errorf("vendoring local deps: resolving %q declared in %q: %w", source, cityTomlPath, err)
		}
		if ok {
			imports = append(imports, imp)
		}
	}
	return imports, nil
}

func appendImportSources(sources []string, imports map[string]config.Import) []string {
	keys := make([]string, 0, len(imports))
	for name := range imports {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		sources = append(sources, imports[name].Source)
	}
	return sources
}

func escapingImportForSource(source, srcPackDir, srcDir string) (escapingImport, bool, error) {
	source = strings.TrimSpace(source)
	if source == "" || strings.HasPrefix(source, "//") || isRemoteImportSource(source) {
		return escapingImport{}, false, nil
	}
	resolved, err := filepath.Abs(filepath.Join(srcPackDir, source))
	if err != nil {
		return escapingImport{}, false, err
	}
	resolved = filepath.Clean(resolved)
	if !pathEscapesRoot(resolved, srcDir) {
		return escapingImport{}, false, nil
	}
	return escapingImport{absSourcePath: resolved, originalSource: source}, true, nil
}

func pathEscapesRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
}

func rewriteImportSources(fs fsys.FS, cityTomlPath string, rewrites map[string]string) error {
	if len(rewrites) == 0 {
		return nil
	}
	info, err := fs.Stat(cityTomlPath)
	if err != nil {
		return fmt.Errorf("vendoring local deps: stating %q: %w", cityTomlPath, err)
	}
	data, err := fs.ReadFile(cityTomlPath)
	if err != nil {
		return fmt.Errorf("vendoring local deps: reading %q: %w", cityTomlPath, err)
	}
	var out []byte
	switch filepath.Base(cityTomlPath) {
	case "city.toml":
		cfg, err := config.Parse(data)
		if err != nil {
			return fmt.Errorf("vendoring local deps: parsing %q: %w", cityTomlPath, err)
		}
		rewriteImportMap(cfg.Imports, rewrites)
		rewriteStringSlice(cfg.Workspace.LegacyIncludes(), cfg.Workspace.SetLegacyIncludes, rewrites)
		rewriteImportMap(cfg.Defaults.Rig.Imports, rewrites)
		for i := range cfg.Rigs {
			rewriteImportMap(cfg.Rigs[i].Imports, rewrites)
			cfg.Rigs[i].Includes = rewriteStrings(cfg.Rigs[i].Includes, rewrites)
		}
		out, err = cfg.Marshal()
		if err != nil {
			return fmt.Errorf("vendoring local deps: marshaling %q: %w", cityTomlPath, err)
		}
	case "pack.toml":
		var cfg config.PackConfig
		if _, err := toml.Decode(string(data), &cfg); err != nil {
			return fmt.Errorf("vendoring local deps: parsing %q: %w", cityTomlPath, err)
		}
		rewriteImportMap(cfg.Imports, rewrites)
		cfg.Pack.Includes = rewriteStrings(cfg.Pack.Includes, rewrites)
		var buf bytes.Buffer
		enc := toml.NewEncoder(&buf)
		enc.Indent = ""
		if err := enc.Encode(&cfg); err != nil {
			return fmt.Errorf("vendoring local deps: marshaling %q: %w", cityTomlPath, err)
		}
		out = buf.Bytes()
	default:
		return nil
	}
	if err := fs.WriteFile(cityTomlPath, out, info.Mode()); err != nil {
		return fmt.Errorf("vendoring local deps: rewriting %q: %w", cityTomlPath, err)
	}
	return nil
}

func rewriteImportMap(imports map[string]config.Import, rewrites map[string]string) {
	for name, imp := range imports {
		if replacement, ok := rewrites[imp.Source]; ok {
			imp.Source = replacement
			imports[name] = imp
		}
	}
}

func rewriteStringSlice(values []string, set func([]string), rewrites map[string]string) {
	set(rewriteStrings(values, rewrites))
}

func rewriteStrings(values []string, rewrites map[string]string) []string {
	if len(values) == 0 {
		return values
	}
	out := append([]string(nil), values...)
	for i, value := range out {
		if replacement, ok := rewrites[value]; ok {
			out[i] = replacement
		}
	}
	return out
}

func copyDirViaFS(fs fsys.FS, src, dst string) error {
	return copyDirViaFSRel(fs, src, dst, "")
}

func copyDirViaFSRel(fs fsys.FS, src, dst, relDir string) error {
	entries, err := fs.ReadDir(src)
	if err != nil {
		return err
	}
	if err := fs.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		relPath := entry.Name()
		if relDir != "" {
			relPath = filepath.Join(relDir, entry.Name())
		}
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if initFromSkip(relPath, true) {
				continue
			}
			if err := copyDirViaFSRel(fs, srcPath, dstPath, relPath); err != nil {
				return err
			}
			continue
		}
		if initFromSkip(relPath, false) {
			continue
		}
		data, err := fs.ReadFile(srcPath)
		if err != nil {
			return err
		}
		info, err := fs.Stat(srcPath)
		if err != nil {
			return err
		}
		if err := fs.WriteFile(dstPath, data, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func chooseVendorName(srcPath string, seen map[string]vendoredPack, cityPath string) string {
	base := filepath.Base(srcPath)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "pack"
	}
	for i := 0; ; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		dst := filepath.Join(cityPath, initVendorDir, name)
		if !vendorDstSeen(dst, seen) {
			return name
		}
	}
}

func vendorDstSeen(dst string, seen map[string]vendoredPack) bool {
	for _, pack := range seen {
		if pack.vendorDst == dst {
			return true
		}
	}
	return false
}
