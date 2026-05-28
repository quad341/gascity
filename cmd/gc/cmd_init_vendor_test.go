package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
)

func TestVendorExternalLocalPackDepsSingleExternalDep(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "source-city")
	cityPath := filepath.Join(dir, "new-city")
	depDir := filepath.Join(dir, "shared", "dolt")

	writeVendorTestFile(t, srcDir, "city.toml", `[workspace]
name = "source"
`)
	packToml := `[pack]
name = "source"
schema = 2

[imports.dolt]
source = "../shared/dolt"
`
	writeVendorTestFile(t, srcDir, "pack.toml", packToml)
	writeVendorTestFile(t, cityPath, "city.toml", `[workspace]
name = "new-city"
`)
	writeVendorTestFile(t, cityPath, "pack.toml", packToml)
	writeVendorTestFile(t, depDir, "pack.toml", `[pack]
name = "dolt"
schema = 2
`)
	writeVendorTestFile(t, depDir, "prompts/worker.md", "run work\n")
	writeVendorTestFile(t, depDir, "skip_test.go", "package skip\n")
	writeVendorTestFile(t, depDir, ".gc/state.json", "{}\n")

	if err := vendorExternalLocalPackDeps(fsys.OSFS{}, srcDir, cityPath); err != nil {
		t.Fatalf("vendorExternalLocalPackDeps: %v", err)
	}

	vendorPack := filepath.Join(cityPath, "packs", "vendor", "dolt", "pack.toml")
	if _, err := os.Stat(vendorPack); err != nil {
		t.Fatalf("vendored pack.toml missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cityPath, "packs", "vendor", "dolt", "prompts", "worker.md")); err != nil {
		t.Fatalf("vendored prompt missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cityPath, "packs", "vendor", "dolt", "skip_test.go")); !os.IsNotExist(err) {
		t.Fatalf("vendored _test.go should be skipped, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(cityPath, "packs", "vendor", "dolt", ".gc")); !os.IsNotExist(err) {
		t.Fatalf("vendored .gc should be skipped, err=%v", err)
	}

	copied := readVendorTestFile(t, filepath.Join(cityPath, "pack.toml"))
	if !strings.Contains(copied, `source = "//packs/vendor/dolt"`) {
		t.Fatalf("copied pack.toml was not rewritten:\n%s", copied)
	}
	source := readVendorTestFile(t, filepath.Join(srcDir, "pack.toml"))
	if !strings.Contains(source, `source = "../shared/dolt"`) {
		t.Fatalf("source pack.toml was modified:\n%s", source)
	}
}

func TestVendorExternalLocalPackDepsNoOpNoExternalDeps(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "source-city")
	cityPath := filepath.Join(dir, "new-city")
	packToml := `[pack]
name = "source"
schema = 2

[imports.local]
source = "packs/local"

[imports.remote]
source = "https://example.com/remote.git"

[imports.cityroot]
source = "//packs/vendor/already"
`
	writeVendorTestFile(t, srcDir, "city.toml", "[workspace]\nname = \"source\"\n")
	writeVendorTestFile(t, srcDir, "pack.toml", packToml)
	writeVendorTestFile(t, srcDir, "packs/local/pack.toml", "[pack]\nname = \"local\"\nschema = 2\n")
	writeVendorTestFile(t, cityPath, "city.toml", "[workspace]\nname = \"new-city\"\n")
	writeVendorTestFile(t, cityPath, "pack.toml", packToml)
	writeVendorTestFile(t, cityPath, "packs/local/pack.toml", "[pack]\nname = \"local\"\nschema = 2\n")

	if err := vendorExternalLocalPackDeps(fsys.OSFS{}, srcDir, cityPath); err != nil {
		t.Fatalf("vendorExternalLocalPackDeps: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cityPath, "packs", "vendor")); !os.IsNotExist(err) {
		t.Fatalf("vendor dir should not exist, err=%v", err)
	}
	if got := readVendorTestFile(t, filepath.Join(cityPath, "pack.toml")); got != packToml {
		t.Fatalf("pack.toml changed unexpectedly:\n%s", got)
	}
}

func TestRewriteImportSourcesCityToml(t *testing.T) {
	dir := t.TempDir()
	cityToml := filepath.Join(dir, "city.toml")
	writeVendorTestFile(t, dir, "city.toml", `[workspace]
name = "source"
includes = ["../legacy"]

[imports.citydep]
source = "../citydep"

[[rigs]]
name = "demo"
path = "/tmp/demo"
includes = ["../rig-legacy"]

[rigs.imports.rigdep]
source = "../rigdep"
`)

	rewrites := map[string]string{
		"../citydep":    "//packs/vendor/citydep",
		"../rigdep":     "//packs/vendor/rigdep",
		"../legacy":     "//packs/vendor/legacy",
		"../rig-legacy": "//packs/vendor/rig-legacy",
	}
	if err := rewriteImportSources(fsys.OSFS{}, cityToml, rewrites); err != nil {
		t.Fatalf("rewriteImportSources: %v", err)
	}
	cfg, err := config.Load(fsys.OSFS{}, cityToml)
	if err != nil {
		t.Fatalf("loading rewritten city.toml: %v", err)
	}
	if got := cfg.Imports["citydep"].Source; got != "//packs/vendor/citydep" {
		t.Fatalf("city import source = %q", got)
	}
	if got := cfg.Rigs[0].Imports["rigdep"].Source; got != "//packs/vendor/rigdep" {
		t.Fatalf("rig import source = %q", got)
	}
	if got := cfg.Workspace.LegacyIncludes(); len(got) != 1 || got[0] != "//packs/vendor/legacy" {
		t.Fatalf("workspace includes = %v", got)
	}
	if got := cfg.Rigs[0].Includes; len(got) != 1 || got[0] != "//packs/vendor/rig-legacy" {
		t.Fatalf("rig includes = %v", got)
	}
}

func TestRewriteImportSourcesPackToml(t *testing.T) {
	dir := t.TempDir()
	packToml := filepath.Join(dir, "pack.toml")
	writeVendorTestFile(t, dir, "pack.toml", `[pack]
name = "source"
schema = 2
includes = ["../legacy"]

[imports.dep]
source = "../dep"
`)

	rewrites := map[string]string{
		"../dep":    "//packs/vendor/dep",
		"../legacy": "//packs/vendor/legacy",
	}
	if err := rewriteImportSources(fsys.OSFS{}, packToml, rewrites); err != nil {
		t.Fatalf("rewriteImportSources: %v", err)
	}
	var cfg config.PackConfig
	if _, err := toml.Decode(readVendorTestFile(t, packToml), &cfg); err != nil {
		t.Fatalf("decoding rewritten pack.toml: %v", err)
	}
	if got := cfg.Imports["dep"].Source; got != "//packs/vendor/dep" {
		t.Fatalf("pack import source = %q", got)
	}
	if got := cfg.Pack.Includes; len(got) != 1 || got[0] != "//packs/vendor/legacy" {
		t.Fatalf("pack includes = %v", got)
	}
}

func writeVendorTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readVendorTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
