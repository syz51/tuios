package layout

// PreviewInsert returns the rectangle a new pane would occupy without
// changing the tree or allocating node/window IDs. It uses the same axis,
// ratio and layout rules as InsertWindow/InsertWindowWithPreselection.
func (t *BSPTree) PreviewInsert(focused int, preselect PreselectionDir, bounds Rect, gap int) Rect {
	const previewID = -1 // Real pane IDs are positive.
	leaf := &TileNode{WindowID: previewID}
	preview := BSPTree{Root: leaf}
	if t != nil && t.Root != nil {
		target := t.FindNode(focused)
		if target == nil {
			target = t.findAnyLeaf()
		}
		if target != nil {
			direction, first := preselect.split()
			ratio := t.splitRatio()
			if direction == SplitNone {
				direction = t.determineAutoSplit(target, bounds, gap)
				ratio = 0.5 // Ordinary insertion's explicit ratio.
			}
			left, right := target, leaf
			if first {
				left, right = right, left
			}
			split := &TileNode{WindowID: -1, SplitType: direction, SplitRatio: ratio, Left: left, Right: right}
			preview.Root = previewReplace(t.Root, target, split)
		}
	}
	return preview.ApplyLayout(bounds, gap)[previewID]
}

// Only the ancestors are copied. Layout reads children and split ratios; it
// neither writes nodes nor follows Parent, so all other subtrees can be shared.
func previewReplace(node, target, replacement *TileNode) *TileNode {
	if node == nil || node == target {
		if node == target {
			return replacement
		}
		return nil
	}
	left := previewReplace(node.Left, target, replacement)
	right := previewReplace(node.Right, target, replacement)
	if left == node.Left && right == node.Right {
		return node
	}
	copy := *node
	copy.Left, copy.Right = left, right
	return &copy
}
