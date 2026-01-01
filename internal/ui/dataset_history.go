package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (ui *UI) showDatasetHistoryDialog(nodeID string) {
	hist, err := ui.controller.GetDatasetHistory(nodeID, 0, 200)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	if len(hist) == 0 {
		dialog.ShowInformation("Dataset History", "No history available for "+nodeID, ui.window)
		return
	}
	list := widget.NewList(
		func() int { return len(hist) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) { o.(*widget.Label).SetText(hist[i]) },
	)
	list.OnSelected = func(id widget.ListItemID) {
		// show full payload in dialog
		d := dialog.NewInformation(fmt.Sprintf("History item %d", id), hist[id], ui.window)
		d.Show()
	}
	dlg := dialog.NewCustom("Dataset History for "+nodeID, "Close", list, ui.window)
	dlg.Resize(fyne.NewSize(800, 480))
	dlg.Show()
}
