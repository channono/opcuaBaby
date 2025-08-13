package ui

import (
	"context"
	"fmt"
	"image/color"
	"net"
	"opcuababy/internal/controller"
	"opcuababy/internal/exporter"
	"opcuababy/internal/opc"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/gopcua/opcua/ua"
)

var (
	// Icon for the root of the tree (the connection itself)
	rootIconResource = fyne.NewStaticResource("root_icon_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
			<circle cx="12" cy="12" r="10" fill="#E8F5E9"/>
			<path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm-1 17.93c-3.94-.49-7-3.85-7-7.93s3.06-7.44 7-7.93v15.86zm2-15.86c3.94.49 7 3.85 7 7.93s-3.06 7.44-7 7.93V4.07z" fill="#16a6ff"/>
		</svg>`))

	// Icon for the Server object node
	serverIconResource = fyne.NewStaticResource("server_icon_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 180 180" width="180" height="180"    >
			<path d="M 45 79 L 90 100.61 L 90 157.39 L 45 179 L 0 157.39 L 0 100.61 Z" fill="#60a917" />
			<path d="M 89 0 L 134 21.61 L 134 78.39 L 89 100 L 44 78.39 L 44 21.61 Z" fill="#a0522d" />
			<path d="M 135 79 L 180 100.61 L 180 154.39 L 135 176 L 90 154.39 L 90 100.61 Z" fill="#1ba1e2" />
		</svg>`))

	// Icon for special/example nodes like "HelloWorld"
	specialIconResource = fyne.NewStaticResource("special_icon_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
			<defs>
				<linearGradient id="starGradient" x1="0%" y1="0%" x2="100%" y2="100%">
				<stop offset="0%" style="stop-color:#FFC107;stop-opacity:1" />
				<stop offset="100%" style="stop-color:#FF5722;stop-opacity:1" />
				</linearGradient>
			</defs>
			<polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" fill="url(#starGradient)"/>
		</svg>`))

	// Icon for Variable nodes (a tag) - Original Color Version
	tagIconResource = fyne.NewStaticResource("tag_icon.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">
		<defs>
			<linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
			<stop offset="0%" stop-color="#1976D2"/>
			<stop offset="100%" stop-color="#64B5F6"/>
			</linearGradient>
		</defs>
		<path d="M10 24 L74 24 L118 68 L68 118 L24 74 Z" fill="url(#g)"/>
		<circle cx="32" cy="42" r="8" fill="#ffffff"/>
		<path d="M68 118 L118 68" stroke="#000" stroke-width="2"/>
		</svg>`))

	// Icon for Method nodes (play symbol)
	methodIconResource = fyne.NewStaticResource("method_icon_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
		<circle cx="12" cy="12" r="10" fill="#E3F2FD"/>
		<polygon points="10 8 16 12 10 16 10 8" fill="#2196F3"/>
		</svg>`))

	// Icon for Object nodes (closed folder)
	objectIconClosedResource = fyne.NewStaticResource("object_closed_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
			<path d="M10 4H4c-1.11 0-2 .89-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z" fill="#FFCA28"/>
		</svg>`))

	// Icon for Object nodes (open folder)
	objectIconOpenResource = fyne.NewStaticResource("object_open_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
			<path d="M20 6h-8l-2-2H4c-1.11 0-1.99.89-1.99 2L2 18c0 1.1.89 2 1.99 2H20c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2zm0 12H4V8h16v10z" fill="#FFCA28"/>
		</svg>`))

	// Icon for View nodes (an eye)
	viewIconResource = fyne.NewStaticResource("view_icon_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
		<path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5C21.27 7.61 17 4.5 12 4.5zm0 10c-2.48 0-4.5-2.02-4.5-4.5S9.52 5.5 12 5.5 16.5 7.52 16.5 10 14.48 14.5 12 14.5z" fill="#64B5F6"/>
		<circle cx="12" cy="10" r="2.5" fill="#1976D2"/>
		</svg>`))

	// Icon for ObjectType and VariableType nodes
	objectTypeIconResource = fyne.NewStaticResource("type_icon_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
			<rect x="3" y="3" width="18" height="18" rx="2" fill="#E8EAF6"/>
			<rect x="3" y="8" width="18" height="2" fill="#7986CB"/>
			<rect x="8" y="10" width="2" height="11" fill="#7986CB"/>
		</svg>`))

	// Icon for DataType nodes
	dataTypeIconResource = fyne.NewStaticResource("datatype_icon_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
			<rect x="4" y="4" width="6" height="6" rx="1" fill="#F48FB1"/>
			<rect x="14" y="4" width="6" height="6" rx="1" fill="#80CBC4"/>
			<rect x="4" y="14" width="6" height="6" rx="1" fill="#90CAF9"/>
			<rect x="14" y="14" width="6" height="6" rx="1" fill="#FFE082"/>
		</svg>`))

	// Icon for ReferenceType nodes
	linkIconResource = fyne.NewStaticResource("link_icon_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
		<defs>
			<linearGradient id="linkGrad" x1="0%" y1="0%" x2="100%" y2="100%">
			<stop offset="0%" style="stop-color:#B0BEC5;stop-opacity:1" />
			<stop offset="100%" style="stop-color:#78909C;stop-opacity:1" />
			</linearGradient>
		</defs>
		<path d="M17 7h-4v-2h4c1.65 0 3 1.35 3 3s-1.35 3-3 3h-4v-2h4c.55 0 1-.45 1-1s-.45-1-1-1zm-8 8H5c-.55 0-1-.45-1-1s.45-1 1-1h4v-2H5c-1.65 0-3 1.35-3 3s1.35 3 3 3h4v-2zm-1-4h6v-2H8v2z" fill="url(#linkGrad)"/>
		</svg>`))

	// Icon for the "Objects" folder node
	objectsFolderIconResource = fyne.NewStaticResource("objects_folder_icon_color.svg", []byte(`
		<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
			<path d="M10 4H4c-1.11 0-2 .89-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z" fill="#2196F3"/>
		</svg>`))
)

type UI struct {
	app        fyne.App
	window     fyne.Window
	controller *controller.Controller

	endpointEntry  *widget.Entry
	connectBtn     *widget.Button
	configBtn      *widget.Button
	exportBtn      *widget.Button
	statusIcon     *widget.Icon
	apiStatusLabel *widget.Label

	config *opc.Config

	nodeTree       *widget.Tree
	nodeLabelByID  map[string]string
	nodeClassByID  map[string]ua.NodeClass
	nodeMetaByID   map[string]string
	nodeCacheMutex sync.RWMutex // 保护上述三个缓存map的读写锁
	selectedNodeID string
	virtualRoot    string

	nodeInfoTable *widget.Table
	nodeInfoData  map[string]string
	nodeInfoKeys  []string

	watchTable             *widget.Table
	watchRows              []*controller.WatchItem
	watchTableMutex        sync.RWMutex
	watchTableColumnWidths map[int]float32 // 缓存订阅表列宽状态

	//lastClickCell widget.TableCellID
	//lastClickTS   time.Time

	selectedWatchRow int
	removeWatchBtn   *widget.Button
	writeWatchBtn    *widget.Button
	watchBtn         *widget.Button
	writeBtn         *widget.Button

	logText    *widget.RichText
	logScroll  *container.Scroll
	logMutex   sync.Mutex
	logBuilder *strings.Builder
}

const maxLogSegments = 15000 // 大约对应几千行日志，可以按需调整

func NewUI(c *controller.Controller, apiStatus *string) *UI {
	a := app.NewWithID("com.giantbaby.opcuababy") // Use App ID for storage
	a.Settings().SetTheme(&compactTheme{})        // Restore the main theme
	w := a.NewWindow("OPC UA Client - Big GiantBaby")
	w.Resize(fyne.NewSize(1200, 800))

	ui := &UI{
		app:                    a,
		window:                 w,
		controller:             c,
		nodeLabelByID:          make(map[string]string),
		nodeClassByID:          make(map[string]ua.NodeClass),
		nodeMetaByID:           make(map[string]string),
		virtualRoot:            "virtualRoot",
		selectedWatchRow:       -1,
		watchRows:              make([]*controller.WatchItem, 0),
		watchTableColumnWidths: make(map[int]float32),
		nodeInfoKeys: []string{
			"NodeID", "NodeClass", "DisplayName",
			"Description", "DataType", "AccessLevel", "Value",
		},
		logBuilder: new(strings.Builder),
		config: &opc.Config{
			EndpointURL:    "opc.tcp://127.0.0.1:4840",
			SecurityPolicy: "Auto",
			SecurityMode:   "Auto",
			AuthMode:       "Anonymous",
			ApplicationURI: "",
			ProductURI:     "",
			SessionTimeout: 30,
			ApiPort:        "8080",
			ApiEnabled:     true,
			ConnectTimeout: 5, // Default 5-second timeout
		},
		apiStatusLabel: widget.NewLabel(*apiStatus),
	}

	ui.loadConfig()

	ui.initWidgets()
	ui.initCallbacks()
	ui.window.SetOnClosed(func() {
		fmt.Println("Window is closing, initiating graceful shutdown...")
		// 1. 发起断开连接的请求。这会触发 controller 去关闭 opcua 客户端。
		//    我们使用 goroutine 是因为它可能是个耗时操作，避免阻塞UI线程。
		go ui.controller.Disconnect()
		// 2. （可选但推荐）给断开操作一点时间来完成。
		//    这个延迟不是必须的，但可以增加后台任务成功退出的几率。
		//    在真实的生产环境中，应该使用更可靠的同步机制，比如 WaitGroup。
		time.Sleep(500 * time.Millisecond)
		// 3. 最终，让 Fyne 应用退出。
		//    注意：Fyne 在 SetOnClosed 后会自动退出，所以这行可能不是必须的，
		//    但明确写出来可以增强代码可读性。
		//    a.Quit()
	})

	go func() {
		for {
			time.Sleep(1 * time.Second)
			fyne.Do(func() {
				ui.apiStatusLabel.SetText(*apiStatus)
			})
		}
	}()

	go func() {
		for parentID := range c.AddressSpaceUpdateChan {
			children := ui.controller.GetAddressSpaceChildren(parentID)
			ui.nodeCacheMutex.Lock()
			for _, cid := range children {
				node := ui.controller.GetNode(cid)
				if node != nil {
					ui.nodeLabelByID[cid] = node.Name
					ui.nodeMetaByID[cid] = ""
					ui.nodeClassByID[cid] = node.NodeClass
				}
			}
			ui.nodeCacheMutex.Unlock()
			fyne.Do(func() {
				ui.nodeTree.Refresh()
			})
		}
	}()
	ui.window.SetContent(ui.makeLayout())
	if ui.config.AutoConnect {
		go func() {
			time.Sleep(500 * time.Millisecond)
			ui.onConnectClicked()
		}()
	}
	return ui
}

func (ui *UI) Run() {
	ui.window.ShowAndRun()
}

func (ui *UI) GetConfig() *opc.Config {
	return ui.config
}

// in internal/ui/ui.go
// in internal/ui/ui.go

func (ui *UI) initWidgets() {
	ui.endpointEntry = widget.NewEntry()
	ui.endpointEntry.SetPlaceHolder("opc.tcp://host:4840 or hostname/IP")
	ui.endpointEntry.SetText(ui.config.EndpointURL)
	ui.endpointEntry.OnChanged = func(s string) {
		ui.config.EndpointURL = s
	}
	ui.endpointEntry.OnSubmitted = func(text string) {
		normalized := normalizeEndpoint(text)
		ui.config.EndpointURL = normalized
		ui.endpointEntry.SetText(normalized)
	}

	ui.connectBtn = widget.NewButtonWithIcon("Connect", theme.LoginIcon(), ui.onConnectClicked)
	ui.configBtn = widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), ui.showConfigDialog)
	ui.exportBtn = widget.NewButtonWithIcon("Export", theme.DownloadIcon(), ui.showExportDialog)

	ui.statusIcon = widget.NewIcon(theme.CancelIcon())

	ui.nodeTree = widget.NewTree(
		ui.treeChildrenCallback,
		ui.treeIsBranchCallback,
		func(isBranch bool) fyne.CanvasObject { return newTreeRow(isBranch, ui) },
		ui.treeUpdateCallback,
	)
	ui.nodeTree.Root = ui.virtualRoot
	ui.selectedNodeID = ""

	ui.nodeTree.OnSelected = func(uid widget.TreeNodeID) {
		ui.selectedNodeID = uid
		if uid == ui.virtualRoot {
			return
		}
		if ui.nodeTree.IsBranch(uid) {
			ui.nodeTree.ToggleBranch(uid)
		}
		go ui.controller.ReadNodeAttributes(string(uid))
	}
	ui.nodeTree.OnUnselected = func(uid widget.TreeNodeID) {
		if ui.selectedNodeID == uid {
			ui.selectedNodeID = ""
			ui.resetNodeDetails()
		}
	}

	ui.nodeInfoData = make(map[string]string)
	ui.nodeInfoTable = widget.NewTable(
		func() (int, int) {
			return len(ui.nodeInfoKeys), 2
		},
		func() fyne.CanvasObject {
			lbl := widget.NewLabel("")
			lbl.Wrapping = fyne.TextWrapWord
			return lbl
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			lbl := obj.(*widget.Label)
			key := ui.nodeInfoKeys[id.Row]
			if id.Col == 0 {
				lbl.SetText(key)
				lbl.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				lbl.SetText(ui.nodeInfoData[key])
				lbl.TextStyle = fyne.TextStyle{}
			}
			lbl.Alignment = fyne.TextAlignLeading
			lbl.Wrapping = fyne.TextWrapWord
			lbl.Refresh()

			if id.Col == 1 {
				lines := len(strings.Split(lbl.Text, "\n"))
				rowHeight := float32(lines) * (theme.TextSize() + 16)
				ui.nodeInfoTable.SetRowHeight(id.Row, rowHeight)
			}
		},
	)
	ui.nodeInfoTable.SetColumnWidth(0, 110)
	ui.nodeInfoTable.SetColumnWidth(1, 200)

	ui.watchTable = widget.NewTable(
		func() (int, int) {
			ui.watchTableMutex.RLock()
			defer ui.watchTableMutex.RUnlock()
			return len(ui.watchRows) + 1, 12
		},
		func() fyne.CanvasObject {
			lbl := widget.NewLabel("")
			rect := canvas.NewRectangle(color.Transparent)
			return container.NewMax(rect, lbl)
		},
		ui.updateWatchTableCell,
	)

	// 设置默认列宽并缓存
	defWidths := []float32{150, 150, 100, 150, 110, 110, 150, 80, 120, 130, 80, 120}
	for i, w := range defWidths {
		ui.watchTable.SetColumnWidth(i, w)
		ui.watchTableColumnWidths[i] = w
	}

	ui.selectedWatchRow = -1
	ui.watchTable.OnSelected = func(id widget.TableCellID) {
		// 单击值列直接弹出写入对话框（提升可用性），避免双击不触发的问题
		if id.Row > 0 && id.Col == 3 {
			row := id.Row - 1
			ui.watchTableMutex.RLock()
			if row >= 0 && row < len(ui.watchRows) {
				item := ui.watchRows[row]
				ui.watchTableMutex.RUnlock()
				go ui.openWriteForNode(item.NodeID)
			} else {
				ui.watchTableMutex.RUnlock()
			}
		}

		if id.Row == 0 {
			ui.selectedWatchRow = -1
			ui.removeWatchBtn.Disable()
			ui.writeWatchBtn.Disable()
			return
		}
		ui.selectedWatchRow = id.Row - 1
		ui.removeWatchBtn.Enable()
		ui.writeWatchBtn.Enable()
		ui.watchTable.Refresh()
	}

	ui.watchBtn = widget.NewButtonWithIcon("Add to Watch", theme.ContentAddIcon(), func() {
		if ui.selectedNodeID != "" {
			ui.controller.AddWatch(string(ui.selectedNodeID))
		}
	})

	// 【已修正】: 清理了所有混乱的旧代码和语法错误，只保留正确的实现。
	ui.writeBtn = widget.NewButtonWithIcon("Write Value", theme.DocumentCreateIcon(), func() {
		if ui.selectedNodeID == "" {
			return
		}
		nid := string(ui.selectedNodeID)
		go ui.openWriteForNode(nid)
	})
	
	ui.watchBtn.Disable()
	ui.writeBtn.Disable()

	ui.removeWatchBtn = widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), func() {
		if ui.selectedWatchRow < 0 || ui.selectedWatchRow >= len(ui.watchRows) {
			return
		}
		nodeID := ui.watchRows[ui.selectedWatchRow].NodeID
		go ui.controller.RemoveWatch(nodeID)
		ui.selectedWatchRow = -1
		ui.removeWatchBtn.Disable()
		ui.writeWatchBtn.Disable()
	})

	ui.removeWatchBtn.Disable()

	ui.writeWatchBtn = widget.NewButtonWithIcon("Write", theme.DocumentCreateIcon(), func() {
		if ui.selectedWatchRow < 0 || ui.selectedWatchRow >= len(ui.watchRows) {
			return
		}
		item := ui.watchRows[ui.selectedWatchRow]
		ui.showWriteDialog(item.NodeID, item.DataType)
	})
	ui.writeWatchBtn.Disable()

	ui.logText = widget.NewRichText()
	ui.logText.Wrapping = fyne.TextWrapOff
	ui.logText.Segments = []widget.RichTextSegment{&widget.TextSegment{Text: "", Style: widget.RichTextStyleInline}}
	ui.logScroll = container.NewScroll(ui.logText)
	ui.logScroll.SetMinSize(fyne.NewSize(200, 150))
}

func (ui *UI) initCallbacks() {
	c := ui.controller
	go func() {
		for msg := range c.LogChan {
			ts := time.Now().Format("15:04:05")
			fullLine := fmt.Sprintf("[%s] %s", ts, msg)

			newSegments := parseColorTags(fullLine)
			// 添加换行，保证每条日志独占一行
			//newSegments = append(newSegments, &widget.TextSegment{Text: "\n", Style: widget.RichTextStyleInline})
			fyne.Do(func() {
				ui.logMutex.Lock()
				defer ui.logMutex.Unlock()
				// 更新可复制的纯文本缓存
				ui.logBuilder.WriteString(fullLine)
				ui.logBuilder.WriteString("\n")
				// 更新富文本（着色）
				ui.logText.Segments = append(ui.logText.Segments, newSegments...)
				if len(ui.logText.Segments) > maxLogSegments {
					startIndex := len(ui.logText.Segments) - (maxLogSegments * 3 / 4)
					ui.logText.Segments = ui.logText.Segments[startIndex:]
				}
				ui.logText.Refresh()
				if ui.logScroll != nil {
					ui.logScroll.ScrollToBottom()
				}
			})
		}
	}()

	c.OnConnectionStateChange = func(connected bool, endpoint string, err error) {
		fyne.Do(func() {
			ui.connectBtn.Enable()
			if connected {
				ui.connectBtn.SetText("Disconnect")
				ui.connectBtn.SetIcon(theme.LogoutIcon())
				ui.statusIcon.SetResource(theme.ConfirmIcon())
				ui.nodeTree.Root = ui.virtualRoot
				ui.nodeTree.OpenBranch(ui.virtualRoot)
			} else {
				ui.connectBtn.SetText("Connect")
				ui.connectBtn.SetIcon(theme.LoginIcon())
				ui.statusIcon.SetResource(theme.CancelIcon())
			}
			ui.statusIcon.Refresh()
		})
	}

	c.OnAddressSpaceReset = func() {
		fyne.Do(func() {
			ui.nodeCacheMutex.Lock()
			ui.nodeLabelByID = make(map[string]string)
			ui.nodeClassByID = make(map[string]ua.NodeClass)
			ui.nodeMetaByID = make(map[string]string)
			ui.nodeCacheMutex.Unlock()

			ui.nodeTree.Root = ""
			ui.nodeTree.Refresh()
		})
	}

	c.OnWatchListUpdate = func(items []*controller.WatchItem) {
		fyne.Do(func() {
			ui.watchTableMutex.Lock()
			ui.watchRows = items
			ui.watchTableMutex.Unlock()
			ui.watchTable.Refresh()
		})
	}

	c.OnNodeAttributesUpdate = func(attrs *controller.NodeAttributes) {
		fyne.Do(func() {
			if attrs == nil {
				ui.resetNodeDetails()
				return
			}

			ui.nodeInfoData = map[string]string{
				"NodeID":      attrs.NodeID,
				"NodeClass":   attrs.NodeClass,
				"DisplayName": attrs.Name,
				"Description": attrs.Description,
				"DataType":    attrs.DataType,
				"AccessLevel": attrs.AccessLevel,
				"Value":       attrs.Value,
			}
			ui.nodeInfoTable.Refresh()

			if strings.Contains(attrs.NodeClass, "Variable") {
                // AccessLevel may be empty on some servers; treat empty as permissive
                if attrs.AccessLevel == "" || strings.Contains(attrs.AccessLevel, "Read") {
                    ui.watchBtn.Enable()
                } else {
                    ui.watchBtn.Disable()
                }
                if attrs.AccessLevel == "" || strings.Contains(attrs.AccessLevel, "Write") {
                    ui.writeBtn.Enable()
                } else {
                    ui.writeBtn.Disable()
                }
            } else {
                ui.watchBtn.Disable()
                ui.writeBtn.Disable()
            }

			ui.nodeCacheMutex.Lock()
			ui.nodeLabelByID[attrs.NodeID] = attrs.Name
			ui.nodeMetaByID[attrs.NodeID] = fmt.Sprintf("%s, %s", attrs.AccessLevel, attrs.DataType)
			ui.nodeCacheMutex.Unlock()
		})
	}
}

func (ui *UI) resetNodeDetails() {
	ui.nodeInfoData = make(map[string]string)
	ui.nodeInfoTable.Refresh()
	ui.watchBtn.Disable()
	ui.writeBtn.Disable()
}

func (ui *UI) onConnectClicked() {
	if ui.connectBtn.Text == "Connect" {
		endpoint := normalizeEndpoint(ui.endpointEntry.Text)
		ui.endpointEntry.SetText(endpoint)
		ui.config.EndpointURL = endpoint
		ui.connectBtn.SetText("Connecting...")
		ui.connectBtn.Disable()
		go ui.controller.Connect(ui.config)
	} else {
		go ui.controller.Disconnect()
	}
}

func (ui *UI) openWriteForNode(nodeID string) {
    // 在后台线程执行网络/读取操作，然后在 UI 线程弹窗，避免跨线程操作 UI 导致崩溃
    go func() {
        // 优先刷新服务器端 DataType
        if a, err := ui.controller.ReadNodeAttributes(nodeID); err == nil && a != nil && a.DataType != "" {
            dt := a.DataType
            fyne.Do(func() {
                ui.showWriteDialog(nodeID, dt)
            })
            return
        }

        // 回退：保留旧 meta 的最后片段
        ui.nodeCacheMutex.RLock()
        dataType := ""
        if meta, ok := ui.nodeMetaByID[nodeID]; ok {
            parts := strings.Split(meta, ",")
            if len(parts) >= 1 {
                dataType = strings.TrimSpace(parts[len(parts)-1])
            }
        }
        ui.nodeCacheMutex.RUnlock()

        dt := dataType
        fyne.Do(func() {
            ui.showWriteDialog(nodeID, dt)
        })
    }()
}

func (ui *UI) showWriteDialog(nodeID, dataType string) {
	valueEntry := widget.NewEntry()
	dialog.ShowForm("Write Value to "+nodeID, "Write", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Data Type", widget.NewLabel(dataType)),
			widget.NewFormItem("New Value", valueEntry),
		},
		func(ok bool) {
			if ok {
				go ui.controller.WriteValue(nodeID, dataType, valueEntry.Text)
			}
		}, ui.window)
}

func (ui *UI) showConfigDialog() {

	endpointEntry := widget.NewEntry()
	endpointEntry.SetText(ui.config.EndpointURL)

	appURIEntry := widget.NewEntry()
	appURIEntry.SetPlaceHolder("urn:hostname:client")
	appURIEntry.SetText(ui.config.ApplicationURI)

	productURIEntry := widget.NewEntry()
	productURIEntry.SetPlaceHolder("urn:your-company:product")
	productURIEntry.SetText(ui.config.ProductURI)

	sessionTimeoutEntry := widget.NewEntry()
	sessionTimeoutEntry.SetPlaceHolder("in seconds")
	sessionTimeoutEntry.SetText(fmt.Sprintf("%d", ui.config.SessionTimeout))

	policySelect := widget.NewSelect(
		[]string{"Auto", "None", "Basic128Rsa15", "Basic256", "Basic256Sha256"},
		nil,
	)
	policySelect.SetSelected(ui.config.SecurityPolicy)

	modeSelect := widget.NewSelect(
		[]string{"Auto", "None", "Sign", "SignAndEncrypt"},
		nil,
	)
	modeSelect.SetSelected(ui.config.SecurityMode)

	authModeRadio := widget.NewRadioGroup([]string{"Anonymous", "Username", "Certificate"}, nil)
	authModeRadio.SetSelected(ui.config.AuthMode)
	authModeRadio.Horizontal = true

	userEntry := widget.NewEntry()
	userEntry.SetPlaceHolder("Username")
	userEntry.SetText(ui.config.Username)
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Password")
	passwordEntry.SetText(ui.config.Password)
	// Remove inner labels to align entries with other form fields for maximum width
	userPassContainer := container.NewVBox(userEntry, passwordEntry)

	certFileEntry := widget.NewEntry()
	certFileEntry.SetPlaceHolder("Client certificate file (.der/.crt)")
	certFileEntry.SetText(ui.config.CertFile)
	certBrowseBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				certFileEntry.SetText(reader.URI().Path())
			}
		}, ui.window)
	})
	certRow := container.NewBorder(nil, nil, nil, certBrowseBtn, certFileEntry)

	keyFileEntry := widget.NewEntry()
	keyFileEntry.SetPlaceHolder("Private key file (.key/.pem)")
	keyFileEntry.SetText(ui.config.KeyFile)
	keyBrowseBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				keyFileEntry.SetText(reader.URI().Path())
			}
		}, ui.window)
	})
	keyRow := container.NewBorder(nil, nil, nil, keyBrowseBtn, keyFileEntry)
	certContainerAll := container.NewVBox(certRow, keyRow)

	// Holder that shows either user/pass or certificate/key
	credHolder := container.NewVBox()
	setCred := func() {
		switch authModeRadio.Selected {
		case "Username":
			credHolder.Objects = []fyne.CanvasObject{userPassContainer}
		case "Certificate":
			credHolder.Objects = []fyne.CanvasObject{certContainerAll}
		default:
			credHolder.Objects = []fyne.CanvasObject{}
		}
		credHolder.Refresh()
	}
	authModeRadio.OnChanged = func(selected string) { setCred() }

	// Initialize credentials area based on current auth mode
	setCred()

	apiPortEntry := widget.NewEntry()
	apiPortEntry.SetPlaceHolder("e.g., 8080")
	apiPortEntry.SetText(ui.config.ApiPort)

	apiEnabledCheck := widget.NewCheck("Enable API/Web Server", nil)
	apiEnabledCheck.SetChecked(ui.config.ApiEnabled)

	autoConnectCheck := widget.NewCheck("Auto-connect on startup", nil)
	autoConnectCheck.SetChecked(ui.config.AutoConnect)

	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetPlaceHolder("in seconds")
	timeoutEntry.SetText(fmt.Sprintf("%.1f", ui.config.ConnectTimeout))

	formItems := []*widget.FormItem{
		widget.NewFormItem("Endpoint URL", endpointEntry),
		widget.NewFormItem("Application URI", appURIEntry),
		widget.NewFormItem("Product URI", productURIEntry),
		widget.NewFormItem("Session Timeout (s)", sessionTimeoutEntry),
		widget.NewFormItem("Connect Timeout (s)", timeoutEntry),
		widget.NewFormItem("Security Policy", policySelect),
		widget.NewFormItem("Security Mode", modeSelect),
		widget.NewFormItem("Authentication", authModeRadio),
		widget.NewFormItem("", credHolder),
		widget.NewFormItem("API Port", apiPortEntry),
		widget.NewFormItem("", apiEnabledCheck),
		widget.NewFormItem("", autoConnectCheck),
	}

	d := dialog.NewForm("Connection Settings", "Save", "Cancel", formItems, func(ok bool) {
		if ok {
			ui.config.EndpointURL = endpointEntry.Text
			ui.endpointEntry.SetText(endpointEntry.Text)
			ui.config.ApplicationURI = appURIEntry.Text
			ui.config.ProductURI = productURIEntry.Text
			ui.config.SecurityPolicy = policySelect.Selected
			ui.config.SecurityMode = modeSelect.Selected
			ui.config.AuthMode = authModeRadio.Selected
			ui.config.Username = userEntry.Text
			ui.config.Password = passwordEntry.Text
			ui.config.CertFile = certFileEntry.Text
			ui.config.KeyFile = keyFileEntry.Text
			ui.config.ApiPort = apiPortEntry.Text
			ui.config.ApiEnabled = apiEnabledCheck.Checked
			ui.config.AutoConnect = autoConnectCheck.Checked

			if timeout, err := strconv.ParseFloat(timeoutEntry.Text, 64); err == nil {
				ui.config.ConnectTimeout = timeout
			}
			if sTimeout, err := strconv.ParseUint(sessionTimeoutEntry.Text, 10, 32); err == nil {
				ui.config.SessionTimeout = uint32(sTimeout)
			}

			ui.saveConfig()
			ui.controller.UpdateApiServerState(ui.config)
		}
	}, ui.window)

	d.Resize(fyne.NewSize(500, 400))
	d.Show()
}

func (ui *UI) updateWatchTableCell(id widget.TableCellID, obj fyne.CanvasObject) {
	ui.watchTableMutex.RLock()
	defer ui.watchTableMutex.RUnlock()

	cont := obj.(*fyne.Container)
	rect := cont.Objects[0].(*canvas.Rectangle)
	lbl := cont.Objects[1].(*widget.Label)

	if id.Row == 0 {
		headers := []string{
			"NodeID", "Name", "DataType", "Value", "Timestamp",
			"Severity", "SymbolicName", "SubCode", "StructChanged", "SemanticsChanged",
			"InfoBits", "RawCode",
		}
		lbl.TextStyle = fyne.TextStyle{Bold: true}
		lbl.SetText(headers[id.Col])
		rect.FillColor = theme.Color(theme.ColorNameFocus)
		obj.Refresh()
		return
	}

	index := id.Row - 1
	if index >= len(ui.watchRows) {
		return
	}
	item := ui.watchRows[index]

	if index == ui.selectedWatchRow {
		rect.FillColor = theme.FocusColor()
	} else {
		rect.FillColor = color.Transparent
	}

	var text string
	switch id.Col {
	case 0:
		text = item.NodeID
	case 1:
		text = item.Name
	case 2:
		text = item.DataType
	case 3:
		text = item.Value
	case 4:
		text = item.Timestamp
	case 5:
		text = item.Severity
	case 6:
		text = item.SymbolicName
	case 7:
		text = strconv.FormatUint(uint64(item.SubCode), 10)
	case 8:
		text = strconv.FormatBool(item.StructureChanged)
	case 9:
		text = strconv.FormatBool(item.SemanticsChanged)
	case 10:
		text = strconv.FormatUint(uint64(item.InfoBits), 10)
	case 11:
		text = item.RawCode
	}

	lbl.TextStyle = fyne.TextStyle{}
	lbl.SetText(text)
	obj.Refresh()

	txt := canvas.NewText(text, color.Transparent)
	txt.TextSize = theme.TextSize()
	neededWidth := txt.MinSize().Width + 20

	curWidth := ui.watchTableColumnWidths[id.Col]
	if neededWidth > curWidth {
		ui.watchTable.SetColumnWidth(id.Col, neededWidth)
		ui.watchTableColumnWidths[id.Col] = neededWidth
	}
}

func (ui *UI) treeChildrenCallback(uid widget.TreeNodeID) []widget.TreeNodeID {
	if uid == ui.virtualRoot {
		return ui.controller.GetAddressSpaceChildren("i=84")
	}
	return ui.controller.GetAddressSpaceChildren(string(uid))
}

func (ui *UI) treeIsBranchCallback(uid widget.TreeNodeID) bool {
    if uid == ui.virtualRoot {
        return true
    }

    ui.nodeCacheMutex.RLock()
    class, ok := ui.nodeClassByID[string(uid)]
    ui.nodeCacheMutex.RUnlock()

    // If class is known and it's a Variable, it's not a branch
    if ok && class == ua.NodeClassVariable {
        return false
    }

    // For unknown class or non-variable classes, optimistically treat as branch
    // and trigger a non-blocking browse to populate children/meta
    if !ui.controller.HasBrowseBeenPerformed(string(uid)) && !ui.controller.IsBrowsing(string(uid)) {
        go ui.controller.Browse(string(uid))
    }

    node := ui.controller.GetNode(string(uid))
    if node != nil {
        return node.HasChildren
    }
    // Unknown yet; allow expansion to feel responsive
    return true
}

func (ui *UI) treeUpdateCallback(uid widget.TreeNodeID, isBranch bool, obj fyne.CanvasObject) {
	tr := obj.(*treeRow)
	tr.nodeID = uid

	ui.nodeCacheMutex.RLock()
	if ncl, ok := ui.nodeClassByID[string(uid)]; ok {
		tr.nodeClass = ncl
	} else {
		tr.nodeClass = ua.NodeClassObject
	}
	ui.nodeCacheMutex.RUnlock()

	tr.isBranch = isBranch
	tr.isOpen = ui.nodeTree.IsBranchOpen(uid)

	ui.nodeCacheMutex.RLock()
	name := ui.nodeLabelByID[string(uid)]
	meta := ui.nodeMetaByID[string(uid)]
	ui.nodeCacheMutex.RUnlock()

	if name == "" {
		if string(uid) == ui.virtualRoot {
			name = "Root"
		} else {
			name = string(uid)
		}
	}
	tr.name.SetText(name)

	if meta != "" {
		tr.meta.SetText(" [" + meta + "]")
	} else {
		tr.meta.SetText("")
	}

	tr.Refresh()
}

type treeRow struct {
	widget.BaseWidget
	nodeID    widget.TreeNodeID
	nodeClass ua.NodeClass
	isBranch  bool
	isOpen    bool
	name      *widget.Label
	meta      *widget.Label
	icon      *widget.Icon
	ui        *UI // Reference to the main UI
}

func newTreeRow(isBranch bool, ui *UI) *treeRow {
	tr := &treeRow{
		isBranch: isBranch,
		name:     widget.NewLabel(""),
		meta:     widget.NewLabel(""),
		icon:     widget.NewIcon(theme.FileIcon()),
		ui:       ui,
	}
	tr.ExtendBaseWidget(tr)
	return tr
}

func (r *treeRow) CreateRenderer() fyne.WidgetRenderer {
	c := container.NewHBox(r.icon, r.name, r.meta)
	return &treeRowRenderer{row: r, objects: []fyne.CanvasObject{c}, layout: c.Layout}
}

type treeRowRenderer struct {
	row     *treeRow
	objects []fyne.CanvasObject
	layout  fyne.Layout
}

func (r *treeRowRenderer) Layout(size fyne.Size)        { r.layout.Layout(r.objects, size) }
func (r *treeRowRenderer) MinSize() fyne.Size           { return r.layout.MinSize(r.objects) }
func (r *treeRowRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *treeRowRenderer) Destroy()                     {}

func (r *treeRowRenderer) Refresh() {
	var iconResource fyne.Resource

	// Get the most up-to-date name directly from the central cache for reliability.
	r.row.ui.nodeCacheMutex.RLock()
	nodeName := r.row.ui.nodeLabelByID[string(r.row.nodeID)]
	r.row.ui.nodeCacheMutex.RUnlock()

	// Highest priority: Check for special cases by ID or the reliable name.
	if r.row.nodeID == r.row.ui.virtualRoot {
		// This is the absolute root of the tree, representing the connection.
		iconResource = rootIconResource
	} else if nodeName == "Objects" {
		iconResource = objectsFolderIconResource
	} else if nodeName == "Server" {
		// This is the standard Server object under Objects.
		iconResource = serverIconResource
	} else if strings.Contains(strings.ToLower(nodeName), "helloworld") {
		iconResource = specialIconResource
	} else {
		// Default logic based on node class and state
		if r.row.isBranch {
			switch r.row.nodeClass {
			case ua.NodeClassObject:
				if r.row.isOpen {
					iconResource = objectIconOpenResource
				} else {
					iconResource = objectIconClosedResource
				}
			case ua.NodeClassView:
				iconResource = viewIconResource
			default:
				// Fallback for other branch types (like ObjectTypes, etc.)
				if r.row.isOpen {
					iconResource = theme.FolderOpenIcon()
				} else {
					iconResource = theme.FolderIcon()
				}
			}
		} else {
			// Logic for leaf nodes
			switch r.row.nodeClass {
			case ua.NodeClassVariable:
				iconResource = tagIconResource
			case ua.NodeClassMethod:
				iconResource = methodIconResource
			case ua.NodeClassObjectType, ua.NodeClassVariableType:
				iconResource = objectTypeIconResource
			case ua.NodeClassReferenceType:
				iconResource = linkIconResource
			case ua.NodeClassDataType:
				iconResource = dataTypeIconResource
			default:
				iconResource = theme.FileIcon()
			}
		}
	}

	r.row.icon.SetResource(iconResource)
	r.row.name.Refresh()
	r.row.meta.Refresh()
	canvas.Refresh(r.row)
}

// Enable right-click (secondary tap) on tree rows for context actions
// This implements fyne.SecondaryTappable
func (r *treeRow) TappedSecondary(ev *fyne.PointEvent) {
    // Do not show menu for virtual root
    if r.nodeID == r.ui.virtualRoot {
        return
    }

    // Build menu item for Add to Watch
    addItem := fyne.NewMenuItem("Add to Watch", func() {
        nid := string(r.nodeID)
        go r.ui.controller.AddWatch(nid)
    })
    // Only enable for Variable nodes
    if r.nodeClass != ua.NodeClassVariable {
        addItem.Disabled = true
    }

    m := fyne.NewMenu("", addItem)
    // Show popup menu (default placement handled by Fyne)
    widget.NewPopUpMenu(m, r.ui.window.Canvas())
}

func (ui *UI) showExportDialog() {
	fileTypeRadio := widget.NewRadioGroup([]string{"JSON", "Excel"}, nil)
	fileTypeRadio.SetSelected("JSON")
	fileTypeRadio.Horizontal = true

	d := dialog.NewForm("Export Address Space", "Export", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Format", fileTypeRadio),
		},
		func(ok bool) {
			if !ok {
				return
			}
			format := fileTypeRadio.Selected
			var filter storage.FileFilter
			var extension string
			if format == "JSON" {
				filter = storage.NewExtensionFileFilter([]string{".json"})
				extension = ".json"
			} else {
				filter = storage.NewExtensionFileFilter([]string{".xlsx"})
				extension = ".xlsx"
			}

			saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
				if err != nil {
					dialog.ShowError(err, ui.window)
					return
				}
				if writer == nil {
					return
				}
				defer writer.Close()

				filePath := writer.URI().Path()
				go ui.runExport(filePath, format)

			}, ui.window)
			saveDialog.SetFileName("export" + extension)
			saveDialog.SetFilter(filter)
			saveDialog.Show()
		}, ui.window)
	d.Show()
}

func (ui *UI) runExport(filePath, format string) {
	client := ui.controller.GetClientForExport()
	if client == nil {
		fyne.CurrentApp().SendNotification(&fyne.Notification{
			Title:   "Export Aborted",
			Content: "Not connected to an OPC UA server.",
		})
		ui.controller.Log("[yellow]Export aborted: not connected.[-]")
		return
	}

	ui.controller.Log(fmt.Sprintf("Starting full address space export to %s...", filePath))
	fyne.CurrentApp().SendNotification(&fyne.Notification{
		Title:   "Export Started",
		Content: "Building full address space. This may take some time...",
	})

	go func() {
		exporter := exporter.New(client)
		var exportErr error

		// Create a context with a long timeout for the entire export process
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if format == "JSON" {
			exportErr = exporter.ExportToJSON(ctx, "i=84", filePath)
		} else {
			exportErr = exporter.ExportToExcel(ctx, "i=84", filePath)
		}

		if exportErr != nil {
			fyne.CurrentApp().SendNotification(&fyne.Notification{
				Title:   "Export Failed",
				Content: exportErr.Error(),
			})
			ui.controller.Log(fmt.Sprintf("[red]Export failed: %v[-]", exportErr))
		} else {
			fyne.CurrentApp().SendNotification(&fyne.Notification{
				Title:   "Export Successful",
				Content: "Address space exported to " + filePath,
			})
			ui.controller.Log(fmt.Sprintf("[green]Successfully exported address space to %s[-]", filePath))
		}
	}()
}

func (ui *UI) makeLayout() fyne.CanvasObject {
	endpointWithStatus := container.NewBorder(nil, nil, nil, ui.statusIcon, ui.endpointEntry)
	connectionCard := widget.NewCard("Endpoint", "",
		container.NewVBox(
			endpointWithStatus,
			container.NewGridWithColumns(3, ui.connectBtn, ui.configBtn, ui.exportBtn),
			ui.apiStatusLabel,
		))

	addressSpaceCard := widget.NewCard("Address Space", "", container.NewScroll(ui.nodeTree))
	leftPanel := container.NewVSplit(connectionCard, addressSpaceCard)
	leftPanel.SetOffset(0.19)
	// Transparent split bar is not supported directly in this Fyne version; fallback to theme override.
	_ = leftPanel

	watchButtons := container.NewHBox(ui.removeWatchBtn, ui.writeWatchBtn,
		widget.NewButtonWithIcon("Clear All", theme.ContentClearIcon(), ui.controller.RemoveAllWatches))
	toolbarBg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	toolbar := container.NewStack(toolbarBg, watchButtons)

	// 恢复默认表格显示，避免覆盖导致内容不可见
	watchScroll := container.NewVScroll(ui.watchTable)
	watchPanel := widget.NewCard("Watch List", "", container.NewBorder(toolbar, nil, nil, nil, watchScroll))

	scroll := container.NewVScroll(ui.nodeInfoTable)
	scroll.SetMinSize(fyne.NewSize(0, 240))

	detailsCard := widget.NewCard("Selected Node Details", "",
		container.NewVBox(
			scroll,
			container.NewGridWithColumns(2, ui.watchBtn, ui.writeBtn),
		),
	)
	clearLogBtn := widget.NewButtonWithIcon("Clear Logs", theme.ContentClearIcon(), ui.clearLogs)
	copyLogBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), ui.copyLogs)
	logTitle := widget.NewLabelWithStyle("Logs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// 顶部标题栏（右侧：复制 + 清空）
	rightBtns := container.NewHBox(copyLogBtn, clearLogBtn)
	header := container.NewBorder(
		nil, nil,
		logTitle,
		rightBtns,
		layout.NewSpacer(),
	)

	logContainer := container.NewBorder(
		header, nil, nil, nil,
		ui.logScroll,
	)

	rightPanel := container.NewVSplit(detailsCard, logContainer)
	rightPanel.SetOffset(0.2)
	// Transparent split bar is not supported directly in this Fyne version; fallback to theme override.
	_ = rightPanel

	centerRightPanel := container.NewHSplit(watchPanel, rightPanel)
	centerRightPanel.SetOffset(0.6)
	// Transparent split bar is not supported directly in this Fyne version; fallback to theme override.
	_ = centerRightPanel

	mainLayout := container.NewHSplit(leftPanel, centerRightPanel)
	mainLayout.SetOffset(0.3)
	// Transparent split bar is not supported directly in this Fyne version; fallback to theme override.
	_ = mainLayout

	return container.NewMax(mainLayout)
}

func (ui *UI) copyLogs() {
	ui.logMutex.Lock()
	text := ui.logBuilder.String()
	ui.logMutex.Unlock()
	// 去掉颜色标记 [color] 和 [-]
	clean := regexp.MustCompile(`\[[a-zA-Z]+\]|\[-\]`).ReplaceAllString(text, "")
	ui.window.Clipboard().SetContent(clean)
}

func (ui *UI) clearLogs() {
	ui.logMutex.Lock()
	ui.logBuilder.Reset()
	ui.logText.Segments = []widget.RichTextSegment{
		&widget.TextSegment{Text: "", Style: widget.RichTextStyleInline},
	}
	ui.logMutex.Unlock()
	fyne.Do(func() {
		ui.logText.Refresh()
		ui.logScroll.ScrollToTop()
	})
}

func normalizeEndpoint(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return "opc.tcp://127.0.0.1:4840"
	}
	if strings.Contains(s, "://") {
		return s
	}
	if strings.HasPrefix(s, "[") {
		if _, _, err := net.SplitHostPort(s); err == nil {
			return "opc.tcp://" + s
		}
		return "opc.tcp://" + s + ":4840"
	}
	if host, port, err := net.SplitHostPort(s); err == nil && host != "" && port != "" {
		return "opc.tcp://" + s
	}
	return "opc.tcp://" + s + ":4840"
}

type compactTheme struct{}

func (t *compactTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	// 更柔和的背景
	if name == theme.ColorNameBackground && variant == theme.VariantLight {
		return color.NRGBA{R: 245, G: 247, B: 250, A: 255}
	}
	// 禁用态更浅
	if name == theme.ColorNameDisabled {
		return color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	}
	// 分割条尽量透明
	if name == theme.ColorNameSeparator {
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0}
	}
	// 阴影去掉，避免分隔条显得厚重
	if name == theme.ColorNameShadow {
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0}
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (t *compactTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}
func (t *compactTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}
func (t *compactTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 4
	case theme.SizeNameText:
		return 12
	case theme.SizeNameInlineIcon:
		return 14
	case theme.SizeNameHeadingText: // 标题文字
		return 14
	default:
		return theme.DefaultTheme().Size(name)
	}
}

const configName = "opcuababy_config.json"

func (ui *UI) saveConfig() {
	data, err := json.MarshalIndent(ui.config, "", "  ")
	if err != nil {
		ui.controller.Log(fmt.Sprintf("Failed to marshal config: %v", err))
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		ui.controller.Log(fmt.Sprintf("Failed to get executable path: %v", err))
		return
	}
	exeDir := filepath.Dir(exePath)
	configFilePath := filepath.Join(exeDir, configName)

	err = os.WriteFile(configFilePath, data, 0644)
	if err != nil {
		ui.controller.Log(fmt.Sprintf("Failed to write config file: %v", err))
		return
	}
}

func (ui *UI) loadConfig() {
	exePath, err := os.Executable()
	if err != nil {
		ui.controller.Log(fmt.Sprintf("Failed to get executable path: %v", err))
		ui.saveConfig() // 兜底保存默认配置到默认保存机制
		return
	}
	exeDir := filepath.Dir(exePath)
	configFilePath := filepath.Join(exeDir, configName)

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		ui.saveConfig()
		return
	}

	err = json.Unmarshal(data, ui.config)
	if err != nil {
		ui.controller.Log(fmt.Sprintf("Failed to unmarshal config: %v", err))
	}
}

var themeColorNameMap = map[string]fyne.ThemeColorName{
	"green":  theme.ColorNameSuccess,
	"red":    theme.ColorNameError,
	"blue":   theme.ColorNamePrimary,
	"yellow": theme.ColorNameWarning,
}

func parseColorTags(logText string) []widget.RichTextSegment {
	tagRegex := regexp.MustCompile(`(\[[a-zA-Z]+\]|\[-\])`)
	matches := tagRegex.FindAllStringIndex(logText, -1)
	var segments []widget.RichTextSegment
	lastIndex := 0
	currentStyle := widget.RichTextStyle{ColorName: ""}
	for _, match := range matches {
		tagStart := match[0]
		tagEnd := match[1]
		if tagStart > lastIndex {
			text := logText[lastIndex:tagStart]
			segments = append(segments, &widget.TextSegment{
				Style: currentStyle,
				Text:  text,
			})
		}
		tag := logText[tagStart:tagEnd]
		if tag == "[-]" {
			currentStyle.ColorName = ""
		} else {
			colorName := strings.Trim(tag, "[]")
			if name, ok := themeColorNameMap[colorName]; ok {
				currentStyle.ColorName = name
			} else {
				currentStyle.ColorName = ""
			}
		}
		lastIndex = tagEnd
	}
	if lastIndex < len(logText) {
		text := logText[lastIndex:]
		segments = append(segments, &widget.TextSegment{
			Style: currentStyle,
			Text:  text,
		})
	}
	return segments
}
