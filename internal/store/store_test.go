package store

import (
	"encoding/json"
	"fmt"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRestartDoesNotReplay(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db")
	s, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	task := engine.Task{ID: "pending", Node: "remote", Status: "running", Created: time.Now(), Request: engine.Request{Kind: "execute"}}
	if e = s.CreateTask(task); e != nil {
		t.Fatal(e)
	}
	s.DB.Close()
	s, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	got, e := s.Task(task.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.Status != "interrupted" {
		t.Fatal(got.Status)
	}
	next, e := s.Next("remote")
	if e != nil || next != nil {
		t.Fatal("replayed interrupted task", e)
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s
}
func TestConcurrentClaimOnlyDispatchesOnce(t *testing.T) {
	s := testStore(t)
	if err := s.CreateTask(engine.Task{ID: "one", Node: "agent", Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "execute"}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan *engine.Task, 20)
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); task, err := s.Next("agent"); results <- task; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	count := 0
	for got := range results {
		if got != nil {
			count++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count != 1 {
		t.Fatalf("dispatched %d times", count)
	}
}
func TestCompletionWinsConcurrentExpiry(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 30; i++ {
		id := fmt.Sprint(i)
		request := engine.Request{Kind: "execute", PlanID: "original-plan"}
		if err := s.CreateTask(engine.Task{ID: id, Node: "agent", Status: "running", Created: time.Now().Add(-4 * time.Minute), Request: request}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		wg.Add(2)
		go func() { defer wg.Done(); errs <- s.ExpireTasks() }()
		go func() {
			defer wg.Done()
			errs <- s.Complete(engine.Task{ID: id, Node: "agent", Status: "succeeded", Result: map[string]int{"deleted": 2}, Request: engine.Request{Kind: "malicious replacement"}})
		}()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.Task(id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "succeeded" || got.Request.PlanID != "original-plan" || got.Error != "" {
			t.Fatalf("lost completed result: %+v", got)
		}
		if err = s.Complete(engine.Task{ID: id, Node: "agent", Status: "failed", Error: "duplicate changed result"}); err != nil {
			t.Fatal(err)
		}
		got, _ = s.Task(id)
		if got.Status != "succeeded" || got.Result == nil {
			t.Fatal("duplicate overwrote result")
		}
	}
}
func TestCompletionRequiresOwnershipAndDispatch(t *testing.T) {
	s := testStore(t)
	original := engine.Task{ID: "owned", Node: "agent", Status: "pending", Created: time.Now()}
	if err := s.CreateTask(original); err != nil {
		t.Fatal(err)
	}
	for _, node := range []string{"agent", "other"} {
		if err := s.Complete(engine.Task{ID: original.ID, Node: node, Status: "succeeded"}); err == nil {
			t.Fatal("accepted an undispatched or foreign task")
		}
	}
	if _, err := s.Claim("owned", "agent"); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(engine.Task{ID: original.ID, Node: "other", Status: "succeeded"}); err == nil {
		t.Fatal("foreign result accepted")
	}
}
func TestCompactScansRetainsSummaryAndLatestTen(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 12; i++ {
		if err := s.CreateTask(engine.Task{ID: fmt.Sprint(i), Node: "agent", Status: "succeeded", Created: time.Now().Add(time.Duration(i) * time.Second), Request: engine.Request{Kind: "scan"}, Result: map[string]int{"bytes": 42}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateTask(engine.Task{ID: "cleanup", Node: "agent", Status: "succeeded", Created: time.Now(), Request: engine.Request{Kind: "execute"}, Result: map[string]int{"deleted": 2}}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompactScans(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		got, err := s.Task(fmt.Sprint(i))
		if err != nil {
			t.Fatal(err)
		}
		if (got.Result == nil) != (i < 2) {
			t.Fatalf("wrong retention for %d", i)
		}
		if got.Status != "succeeded" || got.Error != "" {
			t.Fatal("summary changed")
		}
	}
	got, _ := s.Task("cleanup")
	if got.Result == nil {
		t.Fatal("cleanup audit was removed")
	}
	summaries, err := s.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range summaries {
		if task.Result != nil {
			t.Fatal("large result loaded in listing")
		}
	}
	before, _ := json.Marshal(got)
	if err = s.CompactScans(); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Task("cleanup")
	after, _ := json.Marshal(got)
	if string(before) != string(after) {
		t.Fatal("maintenance changed cleanup")
	}
}

func TestRevocationInterruptsUndeliveredTasks(t *testing.T) {
	s := testStore(t)
	if err := s.AddNode(Node{ID: "remote", Name: "test"}, "hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTask(engine.Task{ID: "queued", Node: "remote", Status: "pending", Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteNode("remote"); err != nil {
		t.Fatal(err)
	}
	if s.Token("remote") != "" {
		t.Fatal("credential not revoked")
	}
	task, _ := s.Task("queued")
	if task.Status != "interrupted" {
		t.Fatal("queue not interrupted")
	}
	next, err := s.Next("remote")
	if err != nil || next != nil {
		t.Fatal("revoked task delivered", err)
	}
}

func TestNodeMetadataSurvivesStaleMetricsAndRestart(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.DB.Close() }()
	stale := Node{ID: "local", Name: "old", LastSeen: time.Now(), Roots: []string{"/var/log"}}
	if err = s.AddNode(stale, "secret-hash"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetNodeMetadata("local", "renamed", "production"); err != nil {
		t.Fatal(err)
	}
	stale.Metrics.Host = "fresh-host"
	if err = s.UpdateNode(stale); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(p)
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.Node("local")
	if err != nil || n.Name != "renamed" || n.Group != "production" || n.Metrics.Host != "fresh-host" || len(n.Roots) != 1 || s.Token("local") != "secret-hash" {
		t.Fatalf("lost metadata/metrics/credentials: %+v %v", n, err)
	}
	if err = s.SetNodeMetadata("missing", "x", ""); err == nil {
		t.Fatal("missing node accepted")
	}
	if err = s.SetNodeMetadata("local", "renamed", ""); err != nil {
		t.Fatal(err)
	}
	nodes, err := s.Nodes()
	if err != nil || len(nodes) != 1 || nodes[0].Group != "" {
		t.Fatalf("cannot ungroup: %+v %v", nodes, err)
	}
}
