package tuie2e

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuitest"
)

// Failure modes, written before implementation:
//   - Checking blanks alone could pass with no divider: inset 0 must paint the
//     same endpoints, and inset 1 must retain the interior strokes and straight trunk.
//   - Moving panes could fake a shorter divider: list-windows rectangles must
//     remain identical across live changes, including restoring inset 0.
//   - A stale separator memo could ignore a live change: exercise 0, 1, 8, 0
//     on the same attached real-PTY client, with a focused pane beside the tee.
//   - Chrome joins or focus hooks could reconnect the trimmed endpoints: read
//     both outer and interior branch ends, with the dock still present.
//   - Trimming only vertical strokes misses horizontal ends: cover both sides
//     of a trunk and a horizontal-only split, with connected inset-0 baselines.
//
// Negative control: see NEGATIVE_CONTROLS.md, "Divider inset".
func TestDividerInset(t *testing.T) {
	t.Run("horizontal-only", func(t *testing.T) {
		base := t.TempDir()
		writeConfig(t, base, "[startup]\ntiled = true\n[appearance]\nshared_borders = true\nborder_style = \"normal\"\ndivider_inset = 0\n")
		const sess = "horizontal"
		term := startIn(t, base, startOpts{cols: 100, rows: 30, args: []string{"new", sess}})
		newWindow(t, term)
		waitWindowCount(t, term, 1, "first pane")
		var first string
		for id := range listRects(t, base, sess) {
			first = id
		}
		var log strings.Builder
		splitSide(t, base, sess, first, "down", &log)
		waitWindowCount(t, term, 2, "horizontal-only split")
		_ = stableText(t, term)
		before := listRects(t, base, sess)
		r := before[first]
		y := r.Y + r.Height
		for _, inset := range []int{0, 1, 8, 0} {
			setLive(t, base, "appearance.divider_inset", fmt.Sprint(inset))
			if err := term.WaitFor(func(s tuitest.Screen) bool {
				if s.Cell(r.X+r.Width/2, y).Content != "─" {
					return false
				}
				for n := 0; n < max(inset, 1); n++ {
					for _, x := range []int{r.X + n, r.X + r.Width - 1 - n} {
						c := s.Cell(x, y).Content
						if inset > 0 {
							if c != " " && c != "" {
								return false
							}
						} else if !strings.ContainsAny(c, "─┌┐└┘") {
							return false
						}
					}
				}
				return true
			}, uiTimeout); err != nil {
				t.Fatalf("horizontal-only inset %d: %v\n%s", inset, err, term.Snapshot())
			}
			if after := listRects(t, base, sess); !reflect.DeepEqual(before, after) {
				t.Fatalf("horizontal-only geometry changed: %+v -> %+v", before, after)
			}
			name := fmt.Sprintf("horizontal-only-%d", inset)
			saveArtifact(t, term, artifactDir(t), name)
			savePNG(t, term.Screen(), hostPalette(t, ""), artifactDir(t), name)
		}
	})

	base := t.TempDir()
	writeConfig(t, base, "[startup]\ntiled = true\n[appearance]\nshared_borders = true\nborder_style = \"normal\"\ndivider_inset = 0\n")
	const sess = "inset"
	term := startIn(t, base, startOpts{cols: 100, rows: 30, args: []string{"new", sess}})
	newWindow(t, term)
	waitWindowCount(t, term, 1, "first pane")
	var first string
	for id := range listRects(t, base, sess) {
		first = id
	}
	var log strings.Builder
	right := splitSide(t, base, sess, first, "right", &log)
	waitWindowCount(t, term, 2, "vertical split")
	splitSide(t, base, sess, right, "down", &log)
	waitWindowCount(t, term, 3, "horizontal split")
	_ = stableText(t, term)
	before := listRects(t, base, sess)
	left := before[first]
	x, top, bottom := left.X+left.Width, left.Y, left.Y+left.Height-1
	y, end := 0, 0
	for _, r := range before {
		if r.X > x {
			y = max(y, r.Y-1)
			end = max(end, r.X+r.Width-1)
		}
	}
	if x <= 0 || y <= top || end <= x {
		t.Fatalf("unexpected split rectangles: %+v", before)
	}
	dir := artifactDir(t)
	for _, inset := range []int{0, 1, 8, 0} {
		setLive(t, base, "appearance.divider_inset", fmt.Sprint(inset))
		failure := ""
		check := func(s tuitest.Screen) bool {
			glyph := func(x, y int) string { return s.Cell(x, y).Content }
			wantTrunk := "│"
			if inset == 0 {
				wantTrunk = "├"
			}
			if glyph(x, top+8) != "│" || glyph(end-8, y) != "─" {
				failure = "interior stroke missing"
				return false
			}
			for n := 0; n < max(inset, 1); n++ {
				for _, p := range [][2]int{{x, top + n}, {x, bottom - n}, {end - n, y}, {x + 1 + n, y}} {
					c := glyph(p[0], p[1])
					if inset > 0 {
						if c != " " && c != "" {
							failure = fmt.Sprintf("endpoint (%d,%d) = %q, want blank", p[0], p[1], c)
							return false
						}
					} else if !strings.ContainsAny(c, "│─┌┐└┘") {
						failure = fmt.Sprintf("baseline endpoint (%d,%d) = %q, want divider", p[0], p[1], c)
						return false
					}
				}
			}
			if glyph(x, y) != wantTrunk {
				failure = fmt.Sprintf("trunk = %q, want %q", glyph(x, y), wantTrunk)
				return false
			}
			return true
		}
		if err := term.WaitFor(check, uiTimeout); err != nil {
			saveArtifact(t, term, dir, fmt.Sprintf("failed-%d", inset))
			savePNG(t, term.Screen(), hostPalette(t, ""), dir, fmt.Sprintf("failed-%d", inset))
			t.Fatalf("inset %d: %s at vertical x=%d y=%d..%d, horizontal y=%d end=%d: %v\n%s", inset, failure, x, top, bottom, y, end, err, term.Snapshot())
		}
		if after := listRects(t, base, sess); !reflect.DeepEqual(before, after) {
			t.Fatalf("inset %d changed geometry: before=%+v after=%+v", inset, before, after)
		}
		name := fmt.Sprintf("inset-%d", inset)
		saveArtifact(t, term, dir, name)
		savePNG(t, term.Screen(), hostPalette(t, ""), dir, name)
	}

	// The other half creates a cross and a horizontal stroke reaching the
	// left content edge. Both branch ends detach; the vertical trunk survives.
	splitSide(t, base, sess, first, "down", &log)
	waitWindowCount(t, term, 4, "cross layout")
	_ = stableText(t, term)
	before = listRects(t, base, sess)
	for _, inset := range []int{1, 8, 0} {
		setLive(t, base, "appearance.divider_inset", fmt.Sprint(inset))
		if err := term.WaitFor(func(s tuitest.Screen) bool {
			wantTrunk := "│"
			if inset == 0 {
				wantTrunk = "┼"
			}
			if s.Cell(x, y).Content != wantTrunk || s.Cell(left.X+8, y).Content != "─" || s.Cell(end-8, y).Content != "─" {
				return false
			}
			for n := 0; n < max(inset, 1); n++ {
				for _, edge := range []int{left.X + n, end - n, x - 1 - n, x + 1 + n} {
					c := s.Cell(edge, y).Content
					if inset > 0 {
						if c != " " && c != "" {
							return false
						}
					} else if c != "─" {
						return false
					}
				}
			}
			return true
		}, uiTimeout); err != nil {
			t.Fatalf("cross/left and right endpoints, inset %d: %v\n%s", inset, err, term.Snapshot())
		}
		if after := listRects(t, base, sess); !reflect.DeepEqual(before, after) {
			t.Fatalf("cross inset changed geometry: %+v -> %+v", before, after)
		}
		name := fmt.Sprintf("cross-%d", inset)
		saveArtifact(t, term, dir, name)
		savePNG(t, term.Screen(), hostPalette(t, ""), dir, name)
	}

	// A partial zoom pans a larger layout beyond the visible content bounds.
	// Trim the visible endpoints, rather than the offscreen split endpoints.
	setLive(t, base, "appearance.zoom_borderless", "false")
	setLive(t, base, "appearance.zoom_size", "90")
	if err := term.SendKeys("z"); err != nil {
		t.Fatal(err)
	}
	_ = stableText(t, term)
	x = -1
	for col := 1; col < 99; col++ {
		count := 0
		for row := 0; row <= bottom; row++ {
			if term.Screen().Cell(col, row).Content == "│" {
				count++
			}
		}
		if count > bottom/2 {
			x = col
			break
		}
	}
	if x < 0 {
		t.Fatalf("partial zoom has no vertical divider\n%s", term.Snapshot())
	}
	before = listRects(t, base, sess)
	setLive(t, base, "appearance.divider_inset", "1")
	if err := term.WaitFor(func(s tuitest.Screen) bool {
		return s.Cell(x, 0).Content == " " && s.Cell(x, bottom).Content == " " && s.Cell(x, bottom/2).Content == "│"
	}, uiTimeout); err != nil {
		t.Fatalf("partial zoom endpoints did not trim: %v\n%s", err, term.Snapshot())
	}
	if after := listRects(t, base, sess); !reflect.DeepEqual(before, after) {
		t.Fatalf("partial zoom inset changed geometry: %+v -> %+v", before, after)
	}
	saveArtifact(t, term, dir, "partial-zoom-1")
	savePNG(t, term.Screen(), hostPalette(t, ""), dir, "partial-zoom-1")
}

// A maximum inset must not erase a divider shorter than two inset lengths.
func TestDividerInsetShort(t *testing.T) {
	base := t.TempDir()
	writeConfig(t, base, "[startup]\ntiled = true\n[appearance]\nshared_borders = true\nborder_style = \"normal\"\ndivider_inset = 8\n")
	const sess = "short-inset"
	term := startIn(t, base, startOpts{cols: 100, rows: 12, args: []string{"new", sess}})
	newWindow(t, term)
	waitWindowCount(t, term, 1, "first short pane")
	var first string
	for id := range listRects(t, base, sess) {
		first = id
	}
	var log strings.Builder
	splitSide(t, base, sess, first, "right", &log)
	waitWindowCount(t, term, 2, "short split")
	_ = stableText(t, term)
	r := listRects(t, base, sess)[first]
	x := r.X + r.Width
	if r.Height <= 1 || r.Height > 16 {
		t.Fatalf("not a short split: %+v", r)
	}
	if err := term.WaitFor(func(s tuitest.Screen) bool {
		count := 0
		for y := r.Y; y < r.Y+r.Height; y++ {
			c := s.Cell(x, y).Content
			if c == "│" {
				count++
			} else if c != " " && c != "" {
				return false
			}
		}
		return count == 1
	}, uiTimeout); err != nil {
		t.Fatalf("short split must retain exactly one straight divider cell: %v\n%s", err, term.Snapshot())
	}
	saveArtifact(t, term, artifactDir(t), "short-inset-8")
	savePNG(t, term.Screen(), hostPalette(t, ""), artifactDir(t), "short-inset-8")
}
