package pool

import (
	"testing"
	"time"
)

// testItem implements Identity for testing.
type testItem struct {
	id           string
	active       bool
	reactivateAt time.Time
}

func (t *testItem) ID() string                        { return t.id }
func (t *testItem) IsActive() bool                    { return t.active }
func (t *testItem) SetActive(v bool)                  { t.active = v }
func (t *testItem) ReactivateAt() time.Time           { return t.reactivateAt }
func (t *testItem) SetReactivateAt(t2 time.Time)      { t.reactivateAt = t2 }

func TestPool_RoundRobin(t *testing.T) {
	items := []Identity{
		&testItem{id: "a", active: true},
		&testItem{id: "b", active: true},
		&testItem{id: "c", active: true},
	}
	p := New(items, DefaultConfig())

	ids := make([]string, 6)
	for i := range ids {
		item, err := p.Next(nil)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = item.ID()
	}

	expected := []string{"a", "b", "c", "a", "b", "c"}
	for i, id := range ids {
		if id != expected[i] {
			t.Fatalf("position %d: expected %s, got %s", i, expected[i], id)
		}
	}
}

func TestPool_SkipInactive(t *testing.T) {
	items := []Identity{
		&testItem{id: "a", active: false},
		&testItem{id: "b", active: true},
		&testItem{id: "c", active: true},
	}
	p := New(items, DefaultConfig())

	item, err := p.Next(nil)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID() != "b" {
		t.Fatalf("expected b, got %s", item.ID())
	}
}

func TestPool_AllInactive(t *testing.T) {
	items := []Identity{
		&testItem{id: "a", active: false},
		&testItem{id: "b", active: false},
	}
	p := New(items, DefaultConfig())

	_, err := p.Next(nil)
	if err == nil {
		t.Fatal("expected error for all inactive")
	}
}

func TestPool_FilterFunc(t *testing.T) {
	items := []Identity{
		&testItem{id: "a", active: true},
		&testItem{id: "b", active: true},
		&testItem{id: "c", active: true},
	}
	p := New(items, DefaultConfig())

	// Filter out "a"
	item, err := p.Next(func(i Identity) bool {
		return i.ID() != "a"
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID() != "b" {
		t.Fatalf("expected b, got %s", item.ID())
	}
}

func TestPool_AutoReactivation(t *testing.T) {
	items := []Identity{
		&testItem{id: "a", active: false, reactivateAt: time.Now().Add(-1 * time.Second)},
	}
	p := New(items, DefaultConfig())

	item, err := p.Next(nil)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID() != "a" {
		t.Fatalf("expected a, got %s", item.ID())
	}
	if !item.IsActive() {
		t.Fatal("expected item to be reactivated")
	}
}

func TestPool_SoftDeactivate(t *testing.T) {
	items := []Identity{
		&testItem{id: "a", active: true},
	}
	p := New(items, DefaultConfig())

	item, _ := p.Next(nil)
	p.SoftDeactivate(item, 1*time.Hour)

	if item.IsActive() {
		t.Fatal("expected item to be deactivated")
	}
	if item.ReactivateAt().IsZero() {
		t.Fatal("expected reactivateAt to be set")
	}
}

func TestPool_Size(t *testing.T) {
	items := []Identity{
		&testItem{id: "a", active: true},
		&testItem{id: "b", active: true},
	}
	p := New(items, DefaultConfig())
	if p.Size() != 2 {
		t.Fatalf("expected 2, got %d", p.Size())
	}

	p.Add(&testItem{id: "c", active: true})
	if p.Size() != 3 {
		t.Fatalf("expected 3, got %d", p.Size())
	}
}
