package gui

import (
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Material Design icons (Apache 2.0) used by the chrome.
var (
	IconBack   = mustIcon(icons.NavigationArrowBack)
	IconHome   = mustIcon(icons.ActionHome)
	IconNew    = mustIcon(icons.ContentAdd)
	IconOpen   = mustIcon(icons.FileFolderOpen)
	IconFolder = mustIcon(icons.FileFolder)
	IconInfo   = mustIcon(icons.ActionInfo)
	IconSave   = mustIcon(icons.ContentSave)
	IconPrint  = mustIcon(icons.ActionPrint)
	IconExport = mustIcon(icons.FileFileUpload)
	IconClose  = mustIcon(icons.NavigationClose)
	IconCheck  = mustIcon(icons.NavigationCheck)
	IconFile   = mustIcon(icons.EditorInsertDriveFile)
	IconText   = mustIcon(icons.ActionDescription)
	IconGrid   = mustIcon(icons.ImageGridOn)
)

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	return ic
}
