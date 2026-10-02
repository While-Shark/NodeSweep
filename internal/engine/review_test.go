package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTrialExplainsAndNeverCreatesExecutablePlan(t *testing.T) {
	root := t.TempDir()
	e := testEngine(root)
	oldFile(t, root, "eligible.log.1")
	oldFile(t, root, "active.log")
	oldFile(t, root, "excluded/old.log.1")
	out, err := e.Run(context.Background(), Request{Kind: "trial", Rule: rule(root)})
	if err != nil {
		t.Fatal(err)
	}
	trial := out.(Plan)
	if trial.ID != "" || len(e.plans) != 0 || trial.Review.Counts["eligible"] != 1 || trial.Review.Counts["active_or_not_archive"] != 1 || trial.Review.Counts["excluded"] != 1 {
		t.Fatalf("%+v", trial)
	}
	if _, err = os.Stat(filepath.Join(root, "eligible.log.1")); err != nil {
		t.Fatal(err)
	}
	plan, err := e.preview(context.Background(), rule(root))
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.execute(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 || len(result.Items) != 1 || result.Items[0].Status != "deleted" || result.Items[0].Bytes != result.Bytes || result.Planned != 1 || result.Finished.Before(result.Started) {
		t.Fatalf("%+v", result)
	}
}
func TestReviewExamplesAreBounded(t *testing.T) {
	root := t.TempDir()
	e := testEngine(root)
	for i := 0; i < 100; i++ {
		oldFile(t, root, ID()+".log")
	}
	out, err := e.review(context.Background(), rule(root), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Review.Examples) != 40 || out.Review.Counts["active_or_not_archive"] != 100 {
		t.Fatalf("%+v", out.Review)
	}
}
