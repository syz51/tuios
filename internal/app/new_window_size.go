package app

import (
	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/layout"
	"github.com/Gaurav-Gosain/tuios/internal/session"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

// newWindowSize predicts the box the placing sync will give a new pane. No
// layout, preselection or identity allocator is changed while predicting it.
// Modes whose placement depends on a later interaction retain the fallback.
func (m *OS) newWindowSize() *session.NewWindowSize {
	if m.PaneWidth() <= 0 || m.PaneHeight() <= 0 || m.daemonWindowIntent || m.zoomedWindow() != nil || m.WorkspaceHasCustom[m.CurrentWorkspace] {
		return nil
	}
	var width, height int
	if !m.AutoTiling {
		_, _, width, height = m.NewWindowPlacement()
	} else if m.UseScrollingLayout {
		// A new pane gets a new column. Resolve that column with the strip's
		// own rules; its position among existing columns cannot change its size.
		sl := layout.NewScrollingLayout()
		sl.DefaultWidth = m.ScrollColumnWidthFraction()
		sl.MaxProportion = float64(m.Settings.GetScrollColumnMax()) / 100
		sl.Gap = m.separatorGap()
		sl.AddColumn(-1)
		r := sl.ComputePositions(m.ScrollingViewWidth(), m.PaneHeight(), m.PaneTop())[-1]
		width, height = r.W, r.H
	} else if m.UseBSPLayout {
		tree := m.WorkspaceTrees[m.CurrentWorkspace]
		if tree == nil {
			tree = layout.NewBSPTree()
			tree.AutoScheme = layout.ParseAutoScheme(m.Settings.TilingScheme)
			if m.Settings.TilingScheme == "" {
				tree.AutoScheme = layout.ParseAutoScheme(config.TilingSchemeSpiral)
			}
		}
		target := m.pendingSplitTarget
		if target == "" {
			if fw := m.GetFocusedWindow(); fw != nil {
				target = fw.ID
			}
		}
		id := m.WindowToBSPID[target]
		if !tree.HasWindow(id) {
			ids := tree.GetAllWindowIDs()
			if len(ids) > 0 {
				id = ids[len(ids)-1]
			}
		}
		r := tree.PreviewInsert(id, m.pendingSplitDir, m.GetBSPBounds(), m.separatorGap())
		width, height = r.W, r.H
	} else {
		panes := m.tilablePanes(m.CurrentWorkspace)
		rects := m.contentTileLayouts(len(panes) + 1)
		if len(rects) == 0 {
			return nil
		}
		r := rects[len(rects)-1]
		width, height = r.Width, r.Height
	}
	// Derive content through the renderer's own frame/inset contract.
	w := terminal.Window{Width: width, Height: height, Tiled: m.panesBorderless()}
	return &session.NewWindowSize{Width: width, Height: height, Cols: w.ContentWidth(), Rows: w.ContentHeight()}
}
