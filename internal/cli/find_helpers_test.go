package cli

import (
	"io"
	"os"
	"testing"

	findsvc "github.com/robertguss/recon/internal/find"
)

func TestFindHelperFunctions(t *testing.T) {
	if got := normalizeFindPath(" ./a/../b.go "); got != "b.go" {
		t.Fatalf("unexpected normalized path %q", got)
	}
	if got := normalizeFindPath("   "); got != "" {
		t.Fatalf("expected empty normalized path, got %q", got)
	}

	if kind, err := normalizeFindKind(""); err != nil || kind != "" {
		t.Fatalf("expected empty kind passthrough, kind=%q err=%v", kind, err)
	}
	if kind, err := normalizeFindKind("METHOD"); err != nil || kind != "method" {
		t.Fatalf("expected method kind normalization, kind=%q err=%v", kind, err)
	}
	if _, err := normalizeFindKind("bad"); err == nil {
		t.Fatal("expected invalid kind error")
	}

	details := map[string]any{}
	addFindFilterDetails(details, findsvc.QueryOptions{PackagePath: "pkg", FilePath: "x.go", Kind: "func"})
	if details["package"] != "pkg" || details["file"] != "x.go" || details["kind"] != "func" {
		t.Fatalf("unexpected details map: %+v", details)
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	printFindFilters(findsvc.QueryOptions{PackagePath: "pkg", FilePath: "x.go", Kind: "func"})
	_ = w.Close()
	os.Stdout = origStdout
	data, _ := io.ReadAll(r)
	_ = r.Close()
	if string(data) == "" {
		t.Fatal("expected filter text output")
	}
}
