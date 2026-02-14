package knowledge

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/recon/internal/db"
)

func setupKnowledgeEnv(t *testing.T) (string, *sql.DB) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Hello(){}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if _, err := db.EnsureReconDir(root); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn, err := db.Open(db.DBPath(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	_, _ = conn.Exec(`INSERT INTO packages(id,path,name,import_path,file_count,line_count,created_at,updated_at) VALUES (1,'.','main','example.com/recon',1,2,'x','x');`)
	_, _ = conn.Exec(`INSERT INTO files(id,package_id,path,language,lines,hash,created_at,updated_at) VALUES (1,1,'main.go','go',2,'h','x','x');`)
	_, _ = conn.Exec(`INSERT INTO symbols(id,file_id,kind,name,signature,body,line_start,line_end,exported,receiver) VALUES (1,1,'func','Hello','func()','func Hello(){}',1,1,1,'');`)

	return root, conn
}

func TestProposeAndVerifyDecisionValidation(t *testing.T) {
	root, conn := setupKnowledgeEnv(t)
	defer conn.Close()
	svc := NewService(conn)

	cases := []ProposeDecisionInput{
		{Reasoning: "r", EvidenceSummary: "e", CheckType: "file_exists", CheckSpec: `{"path":"go.mod"}`, ModuleRoot: root},
		{Title: "t", EvidenceSummary: "e", CheckType: "file_exists", CheckSpec: `{"path":"go.mod"}`, ModuleRoot: root},
		{Title: "t", Reasoning: "r", CheckType: "file_exists", CheckSpec: `{"path":"go.mod"}`, ModuleRoot: root},
		{Title: "t", Reasoning: "r", EvidenceSummary: "e", CheckSpec: `{"path":"go.mod"}`, ModuleRoot: root},
		{Title: "t", Reasoning: "r", EvidenceSummary: "e", CheckType: "file_exists", ModuleRoot: root},
	}
	for i, in := range cases {
		if _, err := svc.ProposeAndVerifyDecision(context.Background(), in); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}

func TestProposeAndVerifyDecisionPromotedAndPending(t *testing.T) {
	root, conn := setupKnowledgeEnv(t)
	defer conn.Close()
	svc := NewService(conn)

	res, err := svc.ProposeAndVerifyDecision(context.Background(), ProposeDecisionInput{
		Title:           "Use Cobra",
		Reasoning:       "Better commands",
		Confidence:      "",
		EvidenceSummary: "go.mod exists",
		CheckType:       "file_exists",
		CheckSpec:       `{"path":"go.mod"}`,
		ModuleRoot:      root,
	})
	if err != nil {
		t.Fatalf("promoted decision error: %v", err)
	}
	if !res.Promoted || !res.VerificationPassed || res.DecisionID == 0 {
		t.Fatalf("expected promoted result, got %+v", res)
	}

	var confidence string
	if err := conn.QueryRow(`SELECT confidence FROM decisions WHERE id = ?;`, res.DecisionID).Scan(&confidence); err != nil {
		t.Fatalf("query confidence: %v", err)
	}
	if confidence != "medium" {
		t.Fatalf("expected default confidence medium, got %q", confidence)
	}

	res2, err := svc.ProposeAndVerifyDecision(context.Background(), ProposeDecisionInput{
		Title:           "Pending one",
		Reasoning:       "will fail",
		Confidence:      "high",
		EvidenceSummary: "missing file",
		CheckType:       "file_exists",
		CheckSpec:       `{"path":"does-not-exist"}`,
		ModuleRoot:      root,
	})
	if err != nil {
		t.Fatalf("pending decision error: %v", err)
	}
	if res2.Promoted || res2.VerificationPassed {
		t.Fatalf("expected pending result, got %+v", res2)
	}

	var status string
	if err := conn.QueryRow(`SELECT status FROM proposals WHERE id = ?;`, res2.ProposalID).Scan(&status); err != nil {
		t.Fatalf("query proposal status: %v", err)
	}
	if status != "pending" {
		t.Fatalf("expected pending proposal status, got %q", status)
	}
}

func TestRunCheckAndCheckImplementations(t *testing.T) {
	root, conn := setupKnowledgeEnv(t)
	defer conn.Close()
	svc := NewService(conn)
	ctx := context.Background()

	if _, err := svc.runCheck(ctx, ProposeDecisionInput{CheckType: "unknown"}); err == nil {
		t.Fatal("expected unsupported check type error")
	}

	if _, err := svc.runFileExists("{", root); err == nil {
		t.Fatal("expected parse error for file_exists")
	}
	if _, err := svc.runFileExists(`{"path":""}`, root); err == nil {
		t.Fatal("expected missing path error for file_exists")
	}
	absPath := filepath.Join(root, "go.mod")
	out, err := svc.runFileExists(`{"path":"`+absPath+`"}`, root)
	if err != nil || !out.Passed {
		t.Fatalf("expected absolute file_exists pass, got out=%+v err=%v", out, err)
	}

	if _, err := svc.runSymbolExists(ctx, "{"); err == nil {
		t.Fatal("expected parse error for symbol_exists")
	}
	if _, err := svc.runSymbolExists(ctx, `{"name":""}`); err == nil {
		t.Fatal("expected missing name error for symbol_exists")
	}
	out, err = svc.runSymbolExists(ctx, `{"name":"Hello"}`)
	if err != nil || !out.Passed {
		t.Fatalf("expected symbol_exists pass, got out=%+v err=%v", out, err)
	}
	out, err = svc.runSymbolExists(ctx, `{"name":"Missing"}`)
	if err != nil || out.Passed {
		t.Fatalf("expected symbol_exists fail, got out=%+v err=%v", out, err)
	}

	if _, err := svc.runGrepPattern("{", root); err == nil {
		t.Fatal("expected parse error for grep_pattern")
	}
	if _, err := svc.runGrepPattern(`{"pattern":""}`, root); err == nil {
		t.Fatal("expected missing pattern error")
	}
	if _, err := svc.runGrepPattern(`{"pattern":"("}`, root); err == nil {
		t.Fatal("expected regex compile error")
	}
	out, err = svc.runGrepPattern(`{"pattern":"package","scope":"*.go"}`, root)
	if err != nil || !out.Passed {
		t.Fatalf("expected grep pattern pass, got out=%+v err=%v", out, err)
	}
	out, err = svc.runGrepPattern(`{"pattern":"no-match","scope":"main.go"}`, root)
	if err != nil || out.Passed {
		t.Fatalf("expected grep pattern fail, got out=%+v err=%v", out, err)
	}
	if _, err := svc.runGrepPattern(`{"pattern":"x"}`, filepath.Join(root, "missing")); err == nil {
		t.Fatal("expected collect files error for bad module root")
	}
}

func TestProposeAndVerifyDecisionDBErrors(t *testing.T) {
	root, conn := setupKnowledgeEnv(t)
	svc := NewService(conn)
	if err := conn.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if _, err := svc.ProposeAndVerifyDecision(context.Background(), ProposeDecisionInput{
		Title:           "x",
		Reasoning:       "r",
		EvidenceSummary: "e",
		CheckType:       "file_exists",
		CheckSpec:       `{"path":"go.mod"}`,
		ModuleRoot:      root,
	}); err == nil || !strings.Contains(err.Error(), "begin decision tx") {
		t.Fatalf("expected begin tx error, got %v", err)
	}

	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, "go.mod"), []byte("module example.com/recon\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if _, err := db.EnsureReconDir(root2); err != nil {
		t.Fatalf("EnsureReconDir: %v", err)
	}
	conn2, err := db.Open(db.DBPath(root2))
	if err != nil {
		t.Fatalf("Open conn2: %v", err)
	}
	defer conn2.Close()
	svc2 := NewService(conn2)
	if _, err := svc2.ProposeAndVerifyDecision(context.Background(), ProposeDecisionInput{
		Title:           "x",
		Reasoning:       "r",
		EvidenceSummary: "e",
		CheckType:       "file_exists",
		CheckSpec:       `{"path":"go.mod"}`,
		ModuleRoot:      root2,
	}); err == nil || !strings.Contains(err.Error(), "insert proposal") {
		t.Fatalf("expected insert proposal error without migrations, got %v", err)
	}
}
