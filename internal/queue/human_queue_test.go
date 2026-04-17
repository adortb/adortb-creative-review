package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adortb/adortb-creative-review/internal/queue"
)

func TestMemQueue_EnqueueAndGet(t *testing.T) {
	q := queue.NewMemQueue()
	ctx := context.Background()

	id, err := q.Enqueue(ctx, &queue.HumanReviewItem{
		CreativeID: 100,
		Priority:   5,
		Reason:     "test reason",
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}

	item, err := q.Get(ctx, id)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if item.Status != queue.StatusPending {
		t.Errorf("expected pending, got %s", item.Status)
	}
	if item.CreativeID != 100 {
		t.Errorf("expected creative_id 100, got %d", item.CreativeID)
	}
}

func TestMemQueue_GetNotFound(t *testing.T) {
	q := queue.NewMemQueue()
	_, err := q.Get(context.Background(), 9999)
	if !errors.Is(err, queue.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemQueue_Resolve(t *testing.T) {
	q := queue.NewMemQueue()
	ctx := context.Background()

	id, _ := q.Enqueue(ctx, &queue.HumanReviewItem{CreativeID: 1})
	err := q.Resolve(ctx, id, queue.ResolveRequest{
		ReviewerID: 42,
		Decision:   "reject",
		Note:       "clearly spam",
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	item, _ := q.Get(ctx, id)
	if item.Status != queue.StatusResolved {
		t.Errorf("expected resolved, got %s", item.Status)
	}
	if item.Decision != "reject" {
		t.Errorf("expected reject decision, got %s", item.Decision)
	}
	if item.ResolvedAt == nil {
		t.Error("expected resolved_at to be set")
	}
}

func TestMemQueue_ResolveAlreadyResolved(t *testing.T) {
	q := queue.NewMemQueue()
	ctx := context.Background()

	id, _ := q.Enqueue(ctx, &queue.HumanReviewItem{CreativeID: 1})
	_ = q.Resolve(ctx, id, queue.ResolveRequest{ReviewerID: 1, Decision: "pass"})

	err := q.Resolve(ctx, id, queue.ResolveRequest{ReviewerID: 2, Decision: "reject"})
	if !errors.Is(err, queue.ErrAlreadyResolved) {
		t.Errorf("expected ErrAlreadyResolved, got %v", err)
	}
}

func TestMemQueue_List(t *testing.T) {
	q := queue.NewMemQueue()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, _ = q.Enqueue(ctx, &queue.HumanReviewItem{CreativeID: int64(i + 1)})
	}

	items, err := q.List(ctx, queue.StatusPending, 3, 0)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items with limit, got %d", len(items))
	}
}

func TestMemQueue_Assign(t *testing.T) {
	q := queue.NewMemQueue()
	ctx := context.Background()

	id, _ := q.Enqueue(ctx, &queue.HumanReviewItem{CreativeID: 1})
	if err := q.Assign(ctx, id, 99); err != nil {
		t.Fatalf("assign failed: %v", err)
	}

	item, _ := q.Get(ctx, id)
	if item.Status != queue.StatusAssigned {
		t.Errorf("expected assigned, got %s", item.Status)
	}
	if item.AssignedTo != 99 {
		t.Errorf("expected assigned_to 99, got %d", item.AssignedTo)
	}
}
