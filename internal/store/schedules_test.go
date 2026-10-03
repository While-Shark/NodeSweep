package store

import (
	"github.com/While-Shark/NodeSweep/internal/engine"
	"testing"
	"time"
)

func TestScheduleStorageCapAndAtomicDisabledRejection(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 32; i++ {
		if err := s.SaveSchedule(Schedule{ID: engine.ID(), Node: "a", Hours: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveSchedule(Schedule{ID: engine.ID(), Node: "a", Hours: 1}); err == nil {
		t.Fatal("schedule limit exceeded")
	}
	list, err := s.Schedules()
	if err != nil || len(list) != 32 {
		t.Fatal(len(list), err)
	}
	v := list[0]
	task := engine.Task{ID: engine.ID(), Node: "a", Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "preview"}}
	v.Enabled = true
	v.Phase = "preview"
	v.Task = task.ID
	if err = s.ScheduleTask(v, task); err == nil {
		t.Fatal("disabled schedule dispatched")
	}
	tasks, err := s.Tasks()
	if err != nil || len(tasks) != 0 {
		t.Fatal("orphan task", tasks, err)
	}
}
