package samplecmd

import (
	"context"
	"errors"
	"testing"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/page"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/sample"
)

type fakeRepository struct {
	items    []sample.Summary
	item     sample.Item
	found    bool
	err      error
	pages    map[string]page.Result[sample.Summary]
	pageErr  map[string]error
	list     int
	requests []page.Request
	gets     int
	lastGet  string
	afterGet func()
}

func (f *fakeRepository) ListPage(_ context.Context, request page.Request) (page.Result[sample.Summary], error) {
	f.list++
	f.requests = append(f.requests, request)
	if err := f.pageErr[request.Token]; err != nil {
		return page.Result[sample.Summary]{}, err
	}
	if f.pages != nil {
		result := f.pages[request.Token]
		result.Items = append([]sample.Summary(nil), result.Items...)
		return result, nil
	}
	return page.Result[sample.Summary]{Items: append([]sample.Summary(nil), f.items...)}, f.err
}

func (f *fakeRepository) Get(_ context.Context, id string) (sample.Item, bool, error) {
	f.gets++
	f.lastGet = id
	if f.afterGet != nil {
		f.afterGet()
	}
	return f.item, f.found, f.err
}

func sampleIntent(command string) operation.Intent {
	return operation.Intent{Command: command, Effect: operation.EffectRead}
}

func TestListPreservesValidatedOpaqueIDs(t *testing.T) {
	const id = "smp_2f4a6c8e0b1d"
	repository := &fakeRepository{items: []sample.Summary{{ID: id, Name: "Alpha"}}}
	items, err := New(repository).List(context.Background(), sampleIntent("sample list"))
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if repository.list != 1 || len(items) != 1 || items[0].ID != id {
		t.Fatalf("calls = %d, items = %+v", repository.list, items)
	}
}

func TestListDrainsEveryPageAndForwardsCursorExactly(t *testing.T) {
	first := sample.Summary{ID: "smp_2f4a6c8e0b1d", Name: "Alpha"}
	second := sample.Summary{ID: "smp_91b3d5f7a2c4", Name: "Beta"}
	repository := &fakeRepository{pages: map[string]page.Result[sample.Summary]{
		"":                {Items: []sample.Summary{first}, NextToken: "Opaque-Cursor_2"},
		"Opaque-Cursor_2": {Items: []sample.Summary{second}},
	}}
	items, err := New(repository).List(context.Background(), sampleIntent("sample list"))
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 || items[0].ID != first.ID || items[1].ID != second.ID {
		t.Fatalf("items = %+v", items)
	}
	if len(repository.requests) != 2 || repository.requests[1].Token != "Opaque-Cursor_2" ||
		repository.requests[0].Size != sampleListBudget.PageSize {
		t.Fatalf("requests = %+v", repository.requests)
	}
}

func TestListReturnsNoPartialItemsWhenLaterPageFails(t *testing.T) {
	repository := &fakeRepository{
		pages: map[string]page.Result[sample.Summary]{
			"": {Items: []sample.Summary{{ID: "smp_2f4a6c8e0b1d", Name: "Alpha"}}, NextToken: "next"},
		},
		pageErr: map[string]error{"next": errors.New("upstream token=secret")},
	}
	items, err := New(repository).List(context.Background(), sampleIntent("sample list"))
	if err == nil || items != nil {
		t.Fatalf("List() items = %+v, error = %v", items, err)
	}
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Kind != fault.KindUnavailable || structured.Code != "page_fetch_failed" {
		t.Fatalf("List() error = %#v", err)
	}
}

func TestListRejectsInvalidAndDuplicateRepositoryData(t *testing.T) {
	const id = "smp_2f4a6c8e0b1d"
	tests := [][]sample.Summary{
		{{ID: "alpha", Name: "Alpha"}},
		{{ID: id, Name: "Alpha"}, {ID: id, Name: "Duplicate"}},
	}
	for _, items := range tests {
		if _, err := New(&fakeRepository{items: items}).List(context.Background(), sampleIntent("sample list")); err == nil {
			t.Errorf("List() accepted %+v", items)
		}
	}
}

func TestReadPassesCanonicalIDUnchanged(t *testing.T) {
	const id = "smp_2f4a6c8e0b1d"
	want := sample.Item{ID: id, Name: "Alpha", Content: "First offline sample."}
	repository := &fakeRepository{item: want, found: true}
	got, err := New(repository).Read(context.Background(), sampleIntent("sample read"), id)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if repository.gets != 1 || repository.lastGet != id || got != want {
		t.Fatalf("gets = %d, last ID = %q, item = %+v", repository.gets, repository.lastGet, got)
	}
}

func TestReadRejectsAmbiguousInputsBeforeRepository(t *testing.T) {
	invalid := []string{
		"",
		"Alpha",
		"smp_2f4a",
		"smp_2f4a6c8e0b1d ",
		"https://example.invalid/smp_2f4a6c8e0b1d",
	}
	for _, id := range invalid {
		repository := &fakeRepository{}
		if _, err := New(repository).Read(context.Background(), sampleIntent("sample read"), id); err == nil {
			t.Errorf("Read(%q) succeeded", id)
		}
		if repository.gets != 0 {
			t.Errorf("Read(%q) called repository %d times", id, repository.gets)
		}
	}
}

func TestReadReportsNotFoundAndRepositoryMismatch(t *testing.T) {
	const id = "smp_2f4a6c8e0b1d"
	if _, err := New(&fakeRepository{}).Read(context.Background(), sampleIntent("sample read"), id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read() error = %v, want ErrNotFound", err)
	}
	_, err := New(&fakeRepository{}).Read(context.Background(), sampleIntent("sample read"), id)
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Kind != fault.KindNotFound || structured.Code != "sample_not_found" ||
		len(structured.NextActions) != 1 || structured.NextActions[0].Command != "sample list" {
		t.Fatalf("structured not-found = %#v", err)
	}

	mismatch := &fakeRepository{
		item:  sample.Item{ID: "smp_91b3d5f7a2c4", Name: "Beta"},
		found: true,
	}
	if _, err := New(mismatch).Read(context.Background(), sampleIntent("sample read"), id); err == nil {
		t.Fatal("Read() accepted a different repository ID")
	}
}

func TestCanceledContextMakesNoRepositoryCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repository := &fakeRepository{}
	service := New(repository)
	if _, err := service.List(ctx, sampleIntent("sample list")); !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v", err)
	}
	if _, err := service.Read(ctx, sampleIntent("sample read"), "smp_2f4a6c8e0b1d"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() error = %v", err)
	}
	if repository.list != 0 || repository.gets != 0 {
		t.Fatalf("repository calls = list %d, get %d", repository.list, repository.gets)
	}
}

func TestReadSuppressesSuccessWhenCanceledDuringRepositoryCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repository := &fakeRepository{
		item:     sample.Item{ID: "smp_2f4a6c8e0b1d", Name: "Alpha"},
		found:    true,
		afterGet: cancel,
	}
	item, err := New(repository).Read(ctx, sampleIntent("sample read"), "smp_2f4a6c8e0b1d")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() error = %v, want context.Canceled", err)
	}
	if item != (sample.Item{}) {
		t.Fatalf("Read() item = %+v, want zero item", item)
	}
	if repository.gets != 1 {
		t.Fatalf("Get() calls = %d, want 1", repository.gets)
	}
}

type stagedCancelContext struct {
	context.Context
	cancel   context.CancelFunc
	checks   int
	cancelAt int
}

func newStagedCancelContext(cancelAt int) *stagedCancelContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &stagedCancelContext{Context: ctx, cancel: cancel, cancelAt: cancelAt}
}

func (c *stagedCancelContext) Err() error {
	c.checks++
	if c.checks == c.cancelAt {
		c.cancel()
	}
	return c.Context.Err()
}

func TestListRechecksCancellationAfterItemValidation(t *testing.T) {
	ctx := newStagedCancelContext(6)
	repository := &fakeRepository{items: []sample.Summary{{ID: "smp_2f4a6c8e0b1d", Name: "Alpha"}}}
	items, err := New(repository).List(ctx, sampleIntent("sample list"))
	if items != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("List() items = %+v, error = %v, want nil and context.Canceled", items, err)
	}
	if repository.list != 1 || ctx.checks != 6 {
		t.Fatalf("repository calls = %d, context checks = %d", repository.list, ctx.checks)
	}
}

func TestCommandsFailClosedForTypedNilRepository(t *testing.T) {
	service := New((*fakeRepository)(nil))
	if items, err := service.List(context.Background(), sampleIntent("sample list")); err == nil || items != nil {
		t.Fatalf("List() items = %+v, error = %v", items, err)
	}
	if item, err := service.Read(context.Background(), sampleIntent("sample read"), "smp_2f4a6c8e0b1d"); err == nil || item != (sample.Item{}) {
		t.Fatalf("Read() item = %+v, error = %v", item, err)
	}
}

func TestSampleCommandsRejectWrongIntentBeforeRepository(t *testing.T) {
	repository := &fakeRepository{}
	service := New(repository)
	wrong := operation.Intent{Command: "doctor", Effect: operation.EffectRead}
	if _, err := service.List(context.Background(), wrong); err == nil {
		t.Fatal("List() accepted doctor intent")
	}
	if _, err := service.Read(context.Background(), wrong, "smp_2f4a6c8e0b1d"); err == nil {
		t.Fatal("Read() accepted doctor intent")
	}
	if repository.list != 0 || repository.gets != 0 {
		t.Fatalf("repository calls = list %d, get %d", repository.list, repository.gets)
	}
}
