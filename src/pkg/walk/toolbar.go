// Copyright 2010 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package walk

import (
	"os"
	"syscall"
	"unsafe"
)

import (
	. "walk/winapi"
	. "walk/winapi/comctl32"
	. "walk/winapi/user32"
)

var toolBarOrigWndProcPtr uintptr
var _ subclassedWidget = &ToolBar{}

type ToolBar struct {
	WidgetBase
	imageList      *ImageList
	actions        *ActionList
	minButtonWidth uint16
	maxButtonWidth uint16
}

func newToolBar(parent Container, style uint) (*ToolBar, os.Error) {
	tb := &ToolBar{}
	tb.actions = newActionList(tb)

	if err := initChildWidget(
		tb,
		parent,
		"ToolbarWindow32",
		CCS_NODIVIDER|style,
		0); err != nil {
		return nil, err
	}

	return tb, nil
}

func NewToolBar(parent Container) (*ToolBar, os.Error) {
	return newToolBar(parent, TBSTYLE_WRAPABLE)
}

func NewVerticalToolBar(parent Container) (*ToolBar, os.Error) {
	return newToolBar(parent, CCS_VERT|CCS_NORESIZE)
}

func (*ToolBar) origWndProcPtr() uintptr {
	return toolBarOrigWndProcPtr
}

func (*ToolBar) setOrigWndProcPtr(ptr uintptr) {
	toolBarOrigWndProcPtr = ptr
}

func (tb *ToolBar) LayoutFlags() LayoutFlags {
	style := GetWindowLong(tb.hWnd, GWL_STYLE)

	if style&CCS_VERT > 0 {
		return ShrinkableVert | GrowableVert | GreedyVert
	}

	// FIXME: Since reimplementation of BoxLayout we must return 0 here,
	// otherwise the ToolBar contained in MainWindow will eat half the space.  
	return 0 //ShrinkableHorz | GrowableHorz
}

func (tb *ToolBar) SizeHint() Size {
	if tb.actions.Len() == 0 {
		return Size{}
	}

	style := GetWindowLong(tb.hWnd, GWL_STYLE)

	if style&CCS_VERT > 0 && tb.minButtonWidth > 0 {
		return Size{int(tb.minButtonWidth), 44}
	}

	// FIXME: Figure out how to do this.
	return Size{44, 44}
}

func (tb *ToolBar) ButtonWidthLimits() (min, max uint16) {
	return tb.minButtonWidth, tb.maxButtonWidth
}

func (tb *ToolBar) SetButtonWidthLimits(min, max uint16) os.Error {
	if SendMessage(tb.hWnd, TB_SETBUTTONWIDTH, 0, uintptr(MAKELONG(min, max))) == 0 {
		return newError("TB_SETBUTTONWIDTH failed")
	}

	tb.minButtonWidth = min
	tb.maxButtonWidth = max

	return nil
}

func (tb *ToolBar) Actions() *ActionList {
	return tb.actions
}

func (tb *ToolBar) ImageList() *ImageList {
	return tb.imageList
}

func (tb *ToolBar) SetImageList(value *ImageList) {
	var hIml HIMAGELIST

	if value != nil {
		hIml = value.hIml
	}

	SendMessage(tb.hWnd, TB_SETIMAGELIST, 0, uintptr(hIml))

	tb.imageList = value
}

func (tb *ToolBar) imageIndex(image *Bitmap) (imageIndex int, err os.Error) {
	imageIndex = -1
	if image != nil {
		// FIXME: Protect against duplicate insertion
		if imageIndex, err = tb.imageList.AddMasked(image); err != nil {
			return
		}
	}

	return
}

func (tb *ToolBar) wndProc(hwnd HWND, msg uint, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_NOTIFY:
		nmm := (*NMMOUSE)(unsafe.Pointer(lParam))

		switch nmm.Hdr.Code {
		case NM_CLICK:
			actionId := uint16(nmm.DwItemSpec)
			if action := actionsById[actionId]; action != nil {
				action.raiseTriggered()
			}
		}
	}

	return tb.WidgetBase.wndProc(hwnd, msg, wParam, lParam)
}

func (tb *ToolBar) initButtonForAction(action *Action, state, style *byte, image *int, text *uintptr) (err os.Error) {
	if tb.hasStyleBits(CCS_VERT) {
		*state |= TBSTATE_WRAP
	} else {
		*style |= BTNS_AUTOSIZE
	}

	if action.checked {
		*state |= TBSTATE_CHECKED
	}

	if action.enabled {
		*state |= TBSTATE_ENABLED
	}

	if action.checkable {
		*style |= BTNS_CHECK
	}

	if action.exclusive {
		*style |= BTNS_GROUP
	}

	if *image, err = tb.imageIndex(action.image); err != nil {
		return
	}

	*text = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(action.Text())))

	return
}

func (tb *ToolBar) onActionChanged(action *Action) os.Error {
	tbbi := TBBUTTONINFO{
		DwMask: TBIF_IMAGE | TBIF_STATE | TBIF_STYLE | TBIF_TEXT,
	}

	tbbi.CbSize = uint(unsafe.Sizeof(tbbi))

	if err := tb.initButtonForAction(
		action,
		&tbbi.FsState,
		&tbbi.FsStyle,
		&tbbi.IImage,
		&tbbi.PszText); err != nil {

		return err
	}

	if 0 == SendMessage(
		tb.hWnd,
		TB_SETBUTTONINFO,
		uintptr(action.id),
		uintptr(unsafe.Pointer(&tbbi))) {

		return newError("SendMessage(TB_SETBUTTONINFO) failed")
	}

	return nil
}

func (tb *ToolBar) onInsertingAction(index int, action *Action) os.Error {
	tbb := TBBUTTON{
		IdCommand: int(action.id),
	}

	if err := tb.initButtonForAction(
		action,
		&tbb.FsState,
		&tbb.FsStyle,
		&tbb.IBitmap,
		&tbb.IString); err != nil {

		return err
	}

	tb.SetVisible(true)

	SendMessage(tb.hWnd, TB_BUTTONSTRUCTSIZE, uintptr(unsafe.Sizeof(tbb)), 0)

	if FALSE == SendMessage(tb.hWnd, TB_ADDBUTTONS, 1, uintptr(unsafe.Pointer(&tbb))) {
		return newError("SendMessage(TB_ADDBUTTONS)")
	}

	SendMessage(tb.hWnd, TB_AUTOSIZE, 0, 0)

	action.addChangedHandler(tb)

	return nil
}

func (tb *ToolBar) removeAt(index int) os.Error {
	action := tb.actions.At(index)
	action.removeChangedHandler(tb)

	if 0 == SendMessage(tb.hWnd, TB_DELETEBUTTON, uintptr(index), 0) {
		return newError("SendMessage(TB_DELETEBUTTON) failed")
	}

	return nil
}

func (tb *ToolBar) onRemovingAction(index int, action *Action) os.Error {
	return tb.removeAt(index)
}

func (tb *ToolBar) onClearingActions() os.Error {
	for i := tb.actions.Len() - 1; i >= 0; i-- {
		if err := tb.removeAt(i); err != nil {
			return err
		}
	}

	return nil
}
