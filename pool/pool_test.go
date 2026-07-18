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

func (t *testItem) ID() string                   { return t.id }
func (t *testItem) IsActive() bool               { return t.active }
func (t *testItem) SetActive(v bool)             { t.active = v }
func (t *testItem) ReactivateAt() time.Time      { return t.reactivateAt }
func (t *testItem) SetReactivateAt(t2 time.Time) { t.reactivateAt = t2 }

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

// testBackoff is a deterministic backoff config (zero jitter) so the growth/cap
// curve can be asserted exactly without flakiness from the jitter term.
var testBackoff = BackoffConfig{
	InitialWait: 5 * time.Minute,
	MaxWait:     30 * time.Minute,
	Multiplier:  2.0,
	JitterPct:   0,
}

// TestSoftDeactivateBackoff_GrowsAndCaps verifies the cooldown grows
// exponentially per trip and saturates at MaxWait (the self-heal ceiling).
func TestSoftDeactivateBackoff_GrowsAndCaps(t *testing.T) {
	cases := []struct {
		trip int
		want time.Duration
	}{
		{1, 5 * time.Minute},
		{2, 10 * time.Minute},
		{3, 20 * time.Minute},
		{4, 30 * time.Minute},  // 40m would exceed cap -> capped
		{5, 30 * time.Minute},  // stays capped
		{10, 30 * time.Minute}, // stays capped
		{0, 5 * time.Minute},   // trip <= 0 treated as trip 1
	}
	for _, c := range cases {
		item := &testItem{id: "a", active: true}
		p := New([]Identity{Identity(item)}, DefaultConfig())
		before := time.Now()
		p.SoftDeactivateBackoff(item, testBackoff, c.trip)
		got := item.ReactivateAt().Sub(before)
		// allow small scheduling slack between time.Now() calls
		if got < c.want-time.Second || got > c.want+time.Second {
			t.Fatalf("trip %d: cooldown = %v, want ~%v", c.trip, got, c.want)
		}
		if item.IsActive() {
			t.Fatalf("trip %d: item should be inactive after SoftDeactivateBackoff", c.trip)
		}
	}
}

// TestPool_AllUnavailable_SelfHeals reproduces the exact operator-observed
// failure: every item in the pool soft-deactivates with a backoff cooldown, so
// Next reports "all N items are unavailable"; then once the cooldown elapses
// (simulated by rewinding ReactivateAt, a fake clock) Next auto-reactivates and
// serves again WITHOUT any pool reconstruction or process restart.
func TestPool_AllUnavailable_SelfHeals(t *testing.T) {
	const n = 3
	items := make([]Identity, n)
	for i := range items {
		items[i] = &testItem{id: string(rune('a' + i)), active: true}
	}
	p := New(items, DefaultConfig())

	// All items hit the transient-failure trip -> backoff soft-deactivation.
	for _, it := range items {
		p.SoftDeactivateBackoff(it, testBackoff, 1)
	}

	// Pool is now exhausted, exactly like the live "all N unavailable" log.
	if _, err := p.Next(nil); err == nil {
		t.Fatal("expected pool to be exhausted while all items are in cooldown")
	}

	// Advance the (fake) clock past the cooldown by rewinding each ReactivateAt
	// into the past -- no time.Sleep, no reconstruction, same Pool instance.
	for _, it := range items {
		it.SetReactivateAt(time.Now().Add(-1 * time.Second))
	}

	got, err := p.Next(nil)
	if err != nil {
		t.Fatalf("pool failed to self-heal after cooldown: %v", err)
	}
	if !got.IsActive() {
		t.Fatal("served item should be reactivated")
	}
	if p.Healthy(nil) == 0 {
		t.Fatal("expected at least one healthy item after self-heal")
	}
}

// TestDeactivateItem_StillPermanent guards against over-correction: the
// permanent path (errSuspended in go-twitter) must stay permanent -- a
// zero-time ReactivateAt that the auto-reactivate guard never clears.
func TestDeactivateItem_StillPermanent(t *testing.T) {
	item := &testItem{id: "a", active: true}
	p := New([]Identity{Identity(item)}, DefaultConfig())

	p.DeactivateItem(item)

	if item.IsActive() {
		t.Fatal("DeactivateItem should leave item inactive")
	}
	if !item.ReactivateAt().IsZero() {
		t.Fatal("DeactivateItem must leave a zero ReactivateAt (permanent)")
	}

	// Even far in the future, Next must never auto-reactivate a permanently
	// deactivated item (the IsZero guard blocks it).
	if _, err := p.Next(nil); err == nil {
		t.Fatal("permanently deactivated item must never be served by Next")
	}
	if item.IsActive() {
		t.Fatal("permanent deactivation must survive a Next sweep")
	}
}

// TestHealthTracker_RecordFailureBoolUnchanged is the backward-compat guard:
// RecordFailure() bool must return the same sequence pre/post the tripCount
// addition for existing (non-opt-in) callers.
func TestHealthTracker_RecordFailureBoolUnchanged(t *testing.T) {
	// maxConsecFailures = 5 -> first 4 failures return false, 5th returns true.
	h := NewHealthTracker(10, 0.8, 5)
	for i := 1; i <= 4; i++ {
		if h.RecordFailure() {
			t.Fatalf("failure %d: RecordFailure returned true too early", i)
		}
	}
	if !h.RecordFailure() {
		t.Fatal("5th consecutive failure: RecordFailure should return true (trip)")
	}
	if h.TripCount() != 1 {
		t.Fatalf("expected TripCount 1 after first trip, got %d", h.TripCount())
	}
	// A success clears the consecutive streak; bool contract resumes false.
	h.RecordSuccess()
	if h.RecordFailure() {
		t.Fatal("after success, single failure should return false again")
	}
}

// TestHealthTracker_TripCountRisingEdge verifies TripCount counts deactivation
// generations (rising-edge transitions), not every threshold-crossing failure:
// a run of failures while latched is one trip; a success re-arms the next trip.
func TestHealthTracker_TripCountRisingEdge(t *testing.T) {
	h := NewHealthTracker(10, 0.8, 5)

	// First trip: 5 consecutive failures.
	for i := 0; i < 5; i++ {
		h.RecordFailure()
	}
	if h.TripCount() != 1 {
		t.Fatalf("after first trip: TripCount = %d, want 1", h.TripCount())
	}
	// More failures while still latched (consec stays >= max, rate stays high):
	// must NOT inflate the generation count.
	for i := 0; i < 5; i++ {
		h.RecordFailure()
	}
	if h.TripCount() != 1 {
		t.Fatalf("failures while latched inflated TripCount to %d, want 1", h.TripCount())
	}
	// Recovery re-arms the latch; the next trip is a new generation.
	h.RecordSuccess()
	for i := 0; i < 5; i++ {
		h.RecordFailure()
	}
	if h.TripCount() != 2 {
		t.Fatalf("after success+re-trip: TripCount = %d, want 2", h.TripCount())
	}
}
