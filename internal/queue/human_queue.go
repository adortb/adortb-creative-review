// Package queue 提供人工审核队列管理。
package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Status 人工审核状态。
type Status string

const (
	StatusPending    Status = "pending"
	StatusAssigned   Status = "assigned"
	StatusResolved   Status = "resolved"
)

// HumanReviewItem 人工审核队列项。
type HumanReviewItem struct {
	ID          int64      `json:"id"`
	CreativeID  int64      `json:"creative_id"`
	AIReviewID  int64      `json:"ai_review_id,omitempty"`
	Priority    int        `json:"priority"`
	Reason      string     `json:"reason"`
	Status      Status     `json:"status"`
	AssignedTo  int64      `json:"assigned_to,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	Decision    string     `json:"decision,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ResolveRequest 人工审核决策请求。
type ResolveRequest struct {
	ReviewerID int64  `json:"reviewer_id"`
	Decision   string `json:"decision"`
	Note       string `json:"note,omitempty"`
}

var ErrNotFound = errors.New("review item not found")
var ErrAlreadyResolved = errors.New("review item already resolved")

// Queue 人工审核队列接口。
type Queue interface {
	Enqueue(ctx context.Context, item *HumanReviewItem) (int64, error)
	List(ctx context.Context, status Status, limit, offset int) ([]*HumanReviewItem, error)
	Get(ctx context.Context, id int64) (*HumanReviewItem, error)
	Resolve(ctx context.Context, id int64, req ResolveRequest) error
	Assign(ctx context.Context, id, reviewerID int64) error
}

// MemQueue 内存实现，用于测试和开发。
type MemQueue struct {
	mu      sync.RWMutex
	items   map[int64]*HumanReviewItem
	nextID  int64
}

// NewMemQueue 创建内存人工队列。
func NewMemQueue() *MemQueue {
	return &MemQueue{items: make(map[int64]*HumanReviewItem)}
}

func (q *MemQueue) Enqueue(_ context.Context, item *HumanReviewItem) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.nextID++
	item.ID = q.nextID
	item.Status = StatusPending
	item.CreatedAt = time.Now()
	// 防止浅拷贝问题
	cp := *item
	q.items[q.nextID] = &cp
	return q.nextID, nil
}

func (q *MemQueue) List(_ context.Context, status Status, limit, offset int) ([]*HumanReviewItem, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	var out []*HumanReviewItem
	for _, item := range q.items {
		if status == "" || item.Status == status {
			cp := *item
			out = append(out, &cp)
		}
	}
	// 简单分页
	if offset >= len(out) {
		return []*HumanReviewItem{}, nil
	}
	end := offset + limit
	if end > len(out) || limit <= 0 {
		end = len(out)
	}
	return out[offset:end], nil
}

func (q *MemQueue) Get(_ context.Context, id int64) (*HumanReviewItem, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	item, ok := q.items[id]
	if !ok {
		return nil, fmt.Errorf("%w: id=%d", ErrNotFound, id)
	}
	cp := *item
	return &cp, nil
}

func (q *MemQueue) Resolve(_ context.Context, id int64, req ResolveRequest) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, ok := q.items[id]
	if !ok {
		return fmt.Errorf("%w: id=%d", ErrNotFound, id)
	}
	if item.Status == StatusResolved {
		return fmt.Errorf("%w: id=%d", ErrAlreadyResolved, id)
	}
	now := time.Now()
	item.Status = StatusResolved
	item.Decision = req.Decision
	item.AssignedTo = req.ReviewerID
	item.ResolvedAt = &now
	return nil
}

func (q *MemQueue) Assign(_ context.Context, id, reviewerID int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, ok := q.items[id]
	if !ok {
		return fmt.Errorf("%w: id=%d", ErrNotFound, id)
	}
	item.Status = StatusAssigned
	item.AssignedTo = reviewerID
	return nil
}
