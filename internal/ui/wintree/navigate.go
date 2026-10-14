package wintree

// Adjacent reports whether r is a neighbour of from in direction nd (vim
// ctrl+w h/j/k/l): r touches from's edge on that side, with positive
// overlap on the other axis (corner-only contact does not count). overlap
// is that overlap.
func Adjacent(from, r Rect, nd NavDir) (overlap int, ok bool) {
	var touches bool
	switch nd {
	case NavLeft:
		touches = r.X+r.W == from.X
		overlap = overlap1D(r.Y, r.H, from.Y, from.H)
	case NavRight:
		touches = from.X+from.W == r.X
		overlap = overlap1D(r.Y, r.H, from.Y, from.H)
	case NavUp:
		touches = r.Y+r.H == from.Y
		overlap = overlap1D(r.X, r.W, from.X, from.W)
	case NavDown:
		touches = from.Y+from.H == r.Y
		overlap = overlap1D(r.X, r.W, from.X, from.W)
	}
	return overlap, touches && overlap > 0
}

// Nearest returns the index in rects of from's neighbour in direction nd:
// among the rects Adjacent to from, the one with the largest overlap,
// the earliest on ties (so callers listing rects in tree or screen order
// get a deterministic pick). ok=false when none is adjacent.
func Nearest(from Rect, rects []Rect, nd NavDir) (int, bool) {
	best, bestOverlap := -1, 0
	for i, r := range rects {
		if ov, ok := Adjacent(from, r, nd); ok && ov > bestOverlap {
			best, bestOverlap = i, ov
		}
	}
	return best, best >= 0
}

// overlap1D returns the overlap length of [a, a+alen) and [b, b+blen),
// negative when disjoint.
func overlap1D(a, alen, b, blen int) int {
	lo := max(a, b)
	hi := min(a+alen, b+blen)
	return hi - lo
}
