package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = orig
	b, _ := os.ReadFile(r.Name())
	_ = r.Close()
	return string(b)
}

func TestWriteJSON(t *testing.T) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	err = writeJSON(map[string]any{"ok": true})
	_ = w.Close()
	os.Stdout = orig
	if err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	if !strings.Contains(buf.String(), "\"ok\": true") {
		t.Fatalf("unexpected json output: %q", buf.String())
	}
}

func TestPromptYesNo(t *testing.T) {
	origIn := os.Stdin
	origErr := os.Stderr
	defer func() {
		os.Stdin = origIn
		os.Stderr = origErr
	}()

	for _, tc := range []struct {
		input      string
		defaultYes bool
		want       bool
	}{
		{"yes\n", false, true},
		{"n\n", true, false},
		{"\n", true, true},
		{"maybe\n", false, false},
	} {
		rIn, wIn, err := os.Pipe()
		if err != nil {
			t.Fatalf("stdin pipe: %v", err)
		}
		rErr, wErr, err := os.Pipe()
		if err != nil {
			t.Fatalf("stderr pipe: %v", err)
		}
		os.Stdin = rIn
		os.Stderr = wErr
		if _, err := wIn.Write([]byte(tc.input)); err != nil {
			t.Fatalf("write stdin: %v", err)
		}
		_ = wIn.Close()

		got, err := promptYesNo("q?", tc.defaultYes)
		_ = wErr.Close()
		_ = rErr.Close()
		_ = rIn.Close()
		if err != nil {
			t.Fatalf("promptYesNo error for %q: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("promptYesNo(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}

	rIn, wIn, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	_ = wIn.Close()
	_ = rIn.Close()
	os.Stdin = rIn
	os.Stderr = wErr
	if _, err := promptYesNo("q?", true); err == nil {
		t.Fatal("expected read error")
	}
	_ = wErr.Close()
	_ = rErr.Close()
}

func TestIsInteractiveTTY(t *testing.T) {
	_ = isInteractiveTTY()
}
