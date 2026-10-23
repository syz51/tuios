package session

// initialWindowSize accepts only complete positive hints. A stale viewport or
// concurrent request may predict a different tile, but it must never allocate
// beyond the session's current usable area (or the kernel's uint16 winsize).
// Preserve the predicted frame allowance when bounding an oversized box.
func initialWindowSize(width, height int, reserve LayoutReserve, hint *NewWindowSize) (int, int, int, int) {
	if hint == nil || hint.Width <= 0 || hint.Height <= 0 || hint.Cols <= 0 || hint.Rows <= 0 || hint.Cols > hint.Width || hint.Rows > hint.Height {
		return width, height, max(width-2, 1), max(height-2, 1)
	}
	w := min(hint.Width, max(width-reserve.Left-reserve.Right, 1), 65535)
	h := min(hint.Height, max(height-reserve.Top-reserve.Bottom, 1), 65535)
	return w, h, max(w-(hint.Width-hint.Cols), 1), max(h-(hint.Height-hint.Rows), 1)
}
