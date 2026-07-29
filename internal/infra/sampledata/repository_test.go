package sampledata

import (
	"context"
	"testing"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/page"
)

func TestRepositoryProvidesStableOpaqueIDs(t *testing.T) {
	repository := New()
	result, err := repository.ListPage(context.Background(), page.Request{Size: 100})
	if err != nil {
		t.Fatalf("ListPage() error = %v", err)
	}
	items := result.Items
	if len(items) != 2 {
		t.Fatalf("List() items = %d, want 2", len(items))
	}
	for _, item := range items {
		if err := item.Validate(); err != nil {
			t.Errorf("item %+v: %v", item, err)
		}
		got, found, err := repository.Get(context.Background(), item.ID)
		if err != nil || !found || got.ID != item.ID || got.Name != item.Name || got.Content == "" {
			t.Errorf("Get(%q) = %+v, %t, %v", item.ID, got, found, err)
		}
	}
}

func TestRepositoryGetUsesExactIDOnly(t *testing.T) {
	repository := New()
	for _, id := range []string{
		"Alpha",
		"smp_2f4a6c8e0b1d ",
		"https://example.invalid/smp_2f4a6c8e0b1d",
	} {
		if _, found, err := repository.Get(context.Background(), id); err != nil || found {
			t.Errorf("Get(%q) found = %t, error = %v", id, found, err)
		}
	}
}

func TestRepositoryListReturnsCopy(t *testing.T) {
	repository := New()
	firstPage, err := repository.ListPage(context.Background(), page.Request{Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	first := firstPage.Items
	first[0].ID = "changed"
	secondPage, err := repository.ListPage(context.Background(), page.Request{Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	second := secondPage.Items
	if second[0].ID == "changed" {
		t.Fatal("List() exposed repository storage")
	}
}

func TestRepositoryListPageOwnsOpaqueCursor(t *testing.T) {
	repository := New()
	first, err := repository.ListPage(context.Background(), page.Request{Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.NextToken == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := repository.ListPage(context.Background(), page.Request{Token: first.NextToken, Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.NextToken != "" || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second page = %+v", second)
	}
}
