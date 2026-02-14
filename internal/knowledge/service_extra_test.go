package knowledge

import (
	"context"
	"testing"
)

func TestRunCheckDispatch(t *testing.T) {
	root, conn := setupKnowledgeEnv(t)
	defer conn.Close()
	svc := NewService(conn)
	ctx := context.Background()

	for _, in := range []ProposeDecisionInput{
		{CheckType: "file_exists", CheckSpec: `{"path":"go.mod"}`, ModuleRoot: root},
		{CheckType: "symbol_exists", CheckSpec: `{"name":"Hello"}`, ModuleRoot: root},
		{CheckType: "grep_pattern", CheckSpec: `{"pattern":"package"}`, ModuleRoot: root},
	} {
		out, err := svc.runCheck(ctx, in)
		if err != nil {
			t.Fatalf("runCheck(%s) error: %v", in.CheckType, err)
		}
		if out.Details == "" {
			t.Fatalf("runCheck(%s) expected details", in.CheckType)
		}
	}
}
