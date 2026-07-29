// Package sampledata provides deterministic offline data for the template's
// discover-to-act example.
package sampledata

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/page"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/sample"
)

// Repository is an in-memory adapter with stable opaque IDs.
type Repository struct {
	items []sample.Item
}

// New creates the default offline repository.
func New() *Repository {
	return &Repository{items: []sample.Item{
		{ID: "smp_2f4a6c8e0b1d", Name: "Alpha", Content: "First offline sample."},
		{ID: "smp_91b3d5f7a2c4", Name: "Beta", Content: "Second offline sample."},
	}}
}

// ListPage returns one stable page without interpreting a caller-provided
// cursor outside this adapter.
func (r *Repository) ListPage(ctx context.Context, request page.Request) (page.Result[sample.Summary], error) {
	if ctx == nil {
		return page.Result[sample.Summary]{}, fmt.Errorf("sample list context is nil")
	}
	if err := ctx.Err(); err != nil {
		return page.Result[sample.Summary]{}, err
	}
	if r == nil {
		return page.Result[sample.Summary]{}, fmt.Errorf("sample repository is nil")
	}
	if err := request.Validate(); err != nil {
		return page.Result[sample.Summary]{}, fmt.Errorf("invalid sample page request: %w", err)
	}
	start := 0
	if request.Token != "" {
		if !strings.HasPrefix(request.Token, "offset:") {
			return page.Result[sample.Summary]{}, fmt.Errorf("invalid sample page token")
		}
		parsed, err := strconv.Atoi(strings.TrimPrefix(request.Token, "offset:"))
		if err != nil || parsed <= 0 || parsed > len(r.items) {
			return page.Result[sample.Summary]{}, fmt.Errorf("invalid sample page token")
		}
		start = parsed
	}
	end := start + request.Size
	if end > len(r.items) {
		end = len(r.items)
	}
	result := page.Result[sample.Summary]{Items: make([]sample.Summary, 0, end-start)}
	for _, item := range r.items[start:end] {
		result.Items = append(result.Items, sample.Summary{ID: item.ID, Name: item.Name})
	}
	if end < len(r.items) {
		result.NextToken = "offset:" + strconv.Itoa(end)
	}
	return result, nil
}

// Get performs exact opaque-ID equality only.
func (r *Repository) Get(ctx context.Context, id string) (sample.Item, bool, error) {
	if ctx == nil {
		return sample.Item{}, false, fmt.Errorf("sample read context is nil")
	}
	if err := ctx.Err(); err != nil {
		return sample.Item{}, false, err
	}
	if r == nil {
		return sample.Item{}, false, fmt.Errorf("sample repository is nil")
	}
	for _, item := range r.items {
		if item.ID == id {
			return item, true, nil
		}
	}
	return sample.Item{}, false, nil
}
