package wintree

import "testing"

// grid3 builds: left column a (full height) | right column b over c.
func grid3(t *testing.T) (*Tree, LeafID, LeafID, LeafID, Rect) {
	t.Helper()
	bounds := Rect{X: 0, Y: 0, W: 180, H: 48}
	tr, a := New(Channel{})
	b, err := tr.Split(a, SplitSideBySide, bounds)
	if err != nil {
		t.Fatal(err)
	}
	c, err := tr.Split(b, SplitStacked, bounds)
	if err != nil {
		t.Fatal(err)
	}
	return tr, a, b, c, bounds
}

// neighbor runs Nearest over the tree's window rects in tree order, the
// way the App walks the message region.
func neighbor(tr *Tree, id LeafID, nd NavDir, bounds Rect) (LeafID, bool) {
	rects := tr.ComputeRects(bounds)
	leaves := tr.Leaves()
	list := make([]Rect, len(leaves))
	for i, l := range leaves {
		list[i] = rects[l]
	}
	i, ok := Nearest(rects[id], list, nd)
	if !ok {
		return id, false
	}
	return leaves[i], true
}

func TestNearest_LeftRight(t *testing.T) {
	tr, a, b, c, bounds := grid3(t)
	if got, ok := neighbor(tr, a, NavRight, bounds); !ok || got != b {
		t.Fatalf("a right = %v %v, want %v (equal overlap: b comes first)", got, ok, b)
	}
	if got, ok := neighbor(tr, b, NavLeft, bounds); !ok || got != a {
		t.Fatalf("b left = %v %v, want %v", got, ok, a)
	}
	if got, ok := neighbor(tr, c, NavLeft, bounds); !ok || got != a {
		t.Fatalf("c left = %v %v, want %v", got, ok, a)
	}
}

func TestNearest_UpDown(t *testing.T) {
	tr, _, b, c, bounds := grid3(t)
	if got, ok := neighbor(tr, b, NavDown, bounds); !ok || got != c {
		t.Fatalf("b down = %v %v, want %v", got, ok, c)
	}
	if got, ok := neighbor(tr, c, NavUp, bounds); !ok || got != b {
		t.Fatalf("c up = %v %v, want %v", got, ok, b)
	}
}

func TestNearest_NoNeighbor(t *testing.T) {
	tr, a, _, _, bounds := grid3(t)
	if got, ok := neighbor(tr, a, NavLeft, bounds); ok || got != a {
		t.Fatalf("a left should report no neighbor, got %v %v", got, ok)
	}
	if _, ok := neighbor(tr, a, NavUp, bounds); ok {
		t.Fatal("a up should report no neighbor")
	}
}

func TestNearest_PicksLargestOverlap(t *testing.T) {
	// left column split into two rows (a top, d bottom); right column
	// b over c. From b going left, a (top row) overlaps b's Y-range
	// more than d does.
	bounds := Rect{X: 0, Y: 0, W: 180, H: 48}
	tr, a := New(Channel{})
	b, _ := tr.Split(a, SplitSideBySide, bounds)
	c, _ := tr.Split(b, SplitStacked, bounds)
	d, _ := tr.Split(a, SplitStacked, bounds)
	if got, ok := neighbor(tr, b, NavLeft, bounds); !ok || got != a {
		t.Fatalf("b left = %v, want %v (top-left window)", got, a)
	}
	if got, ok := neighbor(tr, c, NavLeft, bounds); !ok || got != d {
		t.Fatalf("c left = %v, want %v (bottom-left window)", got, d)
	}
}

func TestAdjacent(t *testing.T) {
	from := Rect{X: 10, Y: 10, W: 10, H: 10}
	cases := []struct {
		name    string
		r       Rect
		nd      NavDir
		wantOv  int
		wantAdj bool
	}{
		{"left, full overlap", Rect{X: 0, Y: 10, W: 10, H: 10}, NavLeft, 10, true},
		{"right, partial overlap", Rect{X: 20, Y: 15, W: 5, H: 20}, NavRight, 5, true},
		{"up", Rect{X: 12, Y: 0, W: 3, H: 10}, NavUp, 3, true},
		{"down", Rect{X: 0, Y: 20, W: 100, H: 1}, NavDown, 10, true},
		{"corner contact only", Rect{X: 0, Y: 0, W: 10, H: 10}, NavLeft, 0, false},
		{"gap between", Rect{X: 0, Y: 10, W: 9, H: 10}, NavLeft, 10, false},
		{"other side", Rect{X: 0, Y: 10, W: 10, H: 10}, NavRight, 10, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ov, ok := Adjacent(from, tc.r, tc.nd)
			if ok != tc.wantAdj || (ok && ov != tc.wantOv) {
				t.Fatalf("Adjacent = %d %v, want %d %v", ov, ok, tc.wantOv, tc.wantAdj)
			}
		})
	}
}
