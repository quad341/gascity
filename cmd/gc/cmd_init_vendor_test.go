package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestVendorExternalLocalPackDepsUsesProvidedFS(t *testing.T) {
	fs := fsys.NewFake()
	for _, dir := range []string{"/src", "/city", "/dep"} {
		fs.Dirs[dir] = true
	}
	fs.Files["/src/city.toml"] = []byte("[workspace]\nname = \"source\"\n")
	fs.Files["/city/city.toml"] = []byte("[workspace]\nname = \"city\"\n")
	packToml := []byte(`[pack]
name = "source"
schema = 2

[imports.dep]
source = "../dep"
`)
	fs.Files["/src/pack.toml"] = packToml
	fs.Files["/city/pack.toml"] = append([]byte(nil), packToml...)
	fs.Files["/dep/pack.toml"] = []byte("[pack]\nname = \"dep\"\nschema = 2\n")

	if err := vendorExternalLocalPackDeps(fs, "/src", "/city"); err != nil {
		t.Fatalf("vendorExternalLocalPackDeps: %v", err)
	}
	if _, ok := fs.Files["/city/packs/vendor/dep/pack.toml"]; !ok {
		t.Fatalf("vendored pack.toml missing from fake FS")
	}
	if got := string(fs.Files["/city/pack.toml"]); !strings.Contains(got, `source = "//packs/vendor/dep"`) {
		t.Fatalf("fake-FS city pack.toml was not rewritten:\n%s", got)
	}
}

func TestVendorExternalLocalPackDepsTransitiveChain(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "source-city")
	cityPath := filepath.Join(dir, "new-city")
	writeVendorTestFile(t, srcDir, "city.toml", "[workspace]\nname = \"source\"\n")
	writeVendorTestFile(t, cityPath, "city.toml", "[workspace]\nname = \"new-city\"\n")
	writeVendorTestFile(t, srcDir, "pack.toml", `[pack]
name = "source"
schema = 2

[imports.a]
source = "../a"
`)
	writeVendorTestFile(t, cityPath, "pack.toml", readVendorTestFile(t, filepath.Join(srcDir, "pack.toml")))
	writeVendorTestFile(t, filepath.Join(dir, "a"), "pack.toml", `[pack]
name = "a"
schema = 2
includes = ["../b"]
`)
	writeVendorTestFile(t, filepath.Join(dir, "b"), "pack.toml", `[pack]
name = "b"
schema = 2

[imports.c]
source = "../c"
`)
	writeVendorTestFile(t, filepath.Join(dir, "c"), "pack.toml", `[pack]
name = "c"
schema = 2
`)

	if err := vendorExternalLocalPackDeps(fsys.OSFS{}, srcDir, cityPath); err != nil {
		t.Fatalf("vendorExternalLocalPackDeps: %v", err)
	}

	for _, name := range []string{"a", "b", "c"} {
		if _, err := os.Stat(filepath.Join(cityPath, "packs", "vendor", name, "pack.toml")); err != nil {
			t.Fatalf("vendored %s pack.toml missing: %v", name, err)
		}
	}
	assertVendorFileContains(t, cityPath, "pack.toml", `source = "//packs/vendor/a"`)
	assertVendorFileContains(t, cityPath, filepath.Join("packs", "vendor", "a", "pack.toml"), `includes = ["//packs/vendor/b"]`)
	assertVendorFileContains(t, cityPath, filepath.Join("packs", "vendor", "b", "pack.toml"), `source = "//packs/vendor/c"`)
}

func TestVendorExternalLocalPackDepsDiamondDedup(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "source-city")
	cityPath := filepath.Join(dir, "new-city")
	writeVendorTestFile(t, srcDir, "city.toml", "[workspace]\nname = \"source\"\n")
	writeVendorTestFile(t, cityPath, "city.toml", "[workspace]\nname = \"new-city\"\n")
	writeVendorTestFile(t, srcDir, "pack.toml", `[pack]
name = "source"
schema = 2

[imports.b]
source = "../b"

[imports.c]
source = "../c"
`)
	writeVendorTestFile(t, cityPath, "pack.toml", readVendorTestFile(t, filepath.Join(srcDir, "pack.toml")))
	for _, name := range []string{"b", "c"} {
		writeVendorTestFile(t, filepath.Join(dir, name), "pack.toml", `[pack]
name = "`+name+`"
schema = 2

[imports.d]
source = "../d"
`)
	}
	writeVendorTestFile(t, filepath.Join(dir, "d"), "pack.toml", `[pack]
name = "d"
schema = 2
`)

	if err := vendorExternalLocalPackDeps(fsys.OSFS{}, srcDir, cityPath); err != nil {
		t.Fatalf("vendorExternalLocalPackDeps: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cityPath, "packs", "vendor", "d", "pack.toml")); err != nil {
		t.Fatalf("vendored d pack.toml missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cityPath, "packs", "vendor", "d-1")); !os.IsNotExist(err) {
		t.Fatalf("diamond dependency should be vendored once, d-1 err=%v", err)
	}
	assertVendorFileContains(t, cityPath, filepath.Join("packs", "vendor", "b", "pack.toml"), `source = "//packs/vendor/d"`)
	assertVendorFileContains(t, cityPath, filepath.Join("packs", "vendor", "c", "pack.toml"), `source = "//packs/vendor/d"`)
}

func TestVendorExternalLocalPackDepsCycleTerminates(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "source-city")
	cityPath := filepath.Join(dir, "new-city")
	writeVendorTestFile(t, srcDir, "city.toml", "[workspace]\nname = \"source\"\n")
	writeVendorTestFile(t, cityPath, "city.toml", "[workspace]\nname = \"new-city\"\n")
	writeVendorTestFile(t, srcDir, "pack.toml", `[pack]
name = "source"
schema = 2

[imports.a]
source = "../a"
`)
	writeVendorTestFile(t, cityPath, "pack.toml", readVendorTestFile(t, filepath.Join(srcDir, "pack.toml")))
	writeVendorTestFile(t, filepath.Join(dir, "a"), "pack.toml", `[pack]
name = "a"
schema = 2

[imports.b]
source = "../b"
`)
	writeVendorTestFile(t, filepath.Join(dir, "b"), "pack.toml", `[pack]
name = "b"
schema = 2

[imports.a]
source = "../a"
`)

	done := make(chan error, 1)
	go func() {
		done <- vendorExternalLocalPackDeps(fsys.OSFS{}, srcDir, cityPath)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("vendorExternalLocalPackDeps: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("vendorExternalLocalPackDeps did not terminate")
	}
	assertVendorFileContains(t, cityPath, filepath.Join("packs", "vendor", "b", "pack.toml"), `source = "//packs/vendor/a"`)
}

func TestVendorExternalLocalPackDepsDuplicateBasenames(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "source-city")
	cityPath := filepath.Join(dir, "new-city")
	writeVendorTestFile(t, srcDir, "city.toml", "[workspace]\nname = \"source\"\n")
	writeVendorTestFile(t, cityPath, "city.toml", "[workspace]\nname = \"new-city\"\n")
	writeVendorTestFile(t, srcDir, "pack.toml", `[pack]
name = "source"
schema = 2

[imports.alpha]
source = "../left/shared"

[imports.beta]
source = "../right/shared"
`)
	writeVendorTestFile(t, cityPath, "pack.toml", readVendorTestFile(t, filepath.Join(srcDir, "pack.toml")))
	writeVendorTestFile(t, filepath.Join(dir, "left", "shared"), "pack.toml", "[pack]\nname = \"left-shared\"\nschema = 2\n")
	writeVendorTestFile(t, filepath.Join(dir, "right", "shared"), "pack.toml", "[pack]\nname = \"right-shared\"\nschema = 2\n")

	if err := vendorExternalLocalPackDeps(fsys.OSFS{}, srcDir, cityPath); err != nil {
		t.Fatalf("vendorExternalLocalPackDeps: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cityPath, "packs", "vendor", "shared", "pack.toml")); err != nil {
		t.Fatalf("vendored shared pack.toml missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cityPath, "packs", "vendor", "shared-1", "pack.toml")); err != nil {
		t.Fatalf("vendored shared-1 pack.toml missing: %v", err)
	}
	copied := readVendorTestFile(t, filepath.Join(cityPath, "pack.toml"))
	if !strings.Contains(copied, `source = "//packs/vendor/shared"`) || !strings.Contains(copied, `source = "//packs/vendor/shared-1"`) {
		t.Fatalf("duplicate basename imports not deconflicted deterministically:\n%s", copied)
	}
}

func TestVendorExternalLocalPackDepsRunawayGraphGuard(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "source-city")
	cityPath := filepath.Join(dir, "new-city")
	writeVendorTestFile(t, srcDir, "city.toml", "[workspace]\nname = \"source\"\n")
	writeVendorTestFile(t, cityPath, "city.toml", "[workspace]\nname = \"new-city\"\n")
	writeVendorTestFile(t, srcDir, "pack.toml", `[pack]
name = "source"
schema = 2

[imports.p00]
source = "../p00"
`)
	writeVendorTestFile(t, cityPath, "pack.toml", readVendorTestFile(t, filepath.Join(srcDir, "pack.toml")))
	for i := 0; i < 51; i++ {
		name := fmt.Sprintf("p%02d", i)
		next := fmt.Sprintf("p%02d", i+1)
		content := fmt.Sprintf("[pack]\nname = %q\nschema = 2\n", name)
		if i < 50 {
			content += fmt.Sprintf("\n[imports.%s]\nsource = \"../%s\"\n", next, next)
		}
		writeVendorTestFile(t, filepath.Join(dir, name), "pack.toml", content)
	}

	err := vendorExternalLocalPackDeps(fsys.OSFS{}, srcDir, cityPath)
	if err == nil {
		t.Fatal("vendorExternalLocalPackDeps succeeded, want graph guard error")
	}
	if !strings.Contains(err.Error(), "dependency graph exceeds") {
		t.Fatalf("error = %v, want dependency graph guard", err)
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

func assertVendorFileContains(t *testing.T, root, rel, want string) {
	t.Helper()
	got := readVendorTestFile(t, filepath.Join(root, rel))
	if !strings.Contains(got, want) {
		t.Fatalf("%s missing %q:\n%s", rel, want, got)
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
