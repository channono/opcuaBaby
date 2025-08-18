package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"net"
	"opcuababy/internal/cert"
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
	"runtime"
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
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

// fontOnlyTheme delegates all visual aspects to the base theme, but overrides Font()
// to use our embedded CJK font on iOS only. This preserves all visuals (colors, sizes,
// icons, separators) while ensuring Chinese glyphs render correctly on iOS builds.
type fontOnlyTheme struct{ base fyne.Theme }

func (t *fontOnlyTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameSeparator:
		return color.Transparent
	case theme.ColorNameShadow:
		return color.Transparent
	case theme.ColorNameHover:
		return color.Transparent
	default:
		return t.base.Color(n, v)
	}
}
func (t *fontOnlyTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return t.base.Icon(n) }
func (t *fontOnlyTheme) Size(n fyne.ThemeSizeName) float32       { return t.base.Size(n) }
func (t *fontOnlyTheme) Font(s fyne.TextStyle) fyne.Resource {
	if runtime.GOOS == "ios" {
		return CJKSubsetFont
	}
	return t.base.Font(s)
}

// Consistent large corner radius to match Apple-style rounded cards across platforms
const (
	appleCornerRadius float32 = 12.0
	maxLogSegments    int     = 15000 // 大约对应几千行日志，可以按需调整
)

// ThemedPanel is a lightweight themed background with a subtle border that
// automatically updates when the app theme changes. No polling or listeners needed:
// Fyne will refresh widgets on theme change, triggering renderer.Refresh.
type ThemedPanel struct {
	widget.BaseWidget
	app          fyne.App
	strokeWidth  float32
	cornerRadius float32
	// fillColor provides the current fill color; defaults to theme.Background
	fillColor func() color.Color
	// inset will shrink the background by this many pixels on all sides
	inset float32
}

func NewThemedPanel(app fyne.App) *ThemedPanel {
	p := &ThemedPanel{app: app, strokeWidth: 1, cornerRadius: appleCornerRadius, inset: 1}
	p.fillColor = func() color.Color { return theme.Color(theme.ColorNameBackground) }
	p.ExtendBaseWidget(p)
	return p
}

// NewThemedBackground returns a borderless themed background that still
// responds to theme change events (used for the outermost background).
func NewThemedBackground(app fyne.App) *ThemedPanel {
	p := &ThemedPanel{app: app, strokeWidth: 0, cornerRadius: 0, inset: 0}
	p.fillColor = func() color.Color { return theme.Color(theme.ColorNameBackground) }
	p.ExtendBaseWidget(p)
	return p
}

// NewThemedArea allows specifying a dynamic fill color function (e.g., using theme.Color(...)),
// plus border width and corner radius.
func NewThemedArea(app fyne.App, fill func() color.Color, strokeWidth, cornerRadius float32) *ThemedPanel {
	p := &ThemedPanel{app: app, strokeWidth: strokeWidth, cornerRadius: cornerRadius, inset: 0}
	p.fillColor = fill
	p.ExtendBaseWidget(p)
	return p
}

func (p *ThemedPanel) CreateRenderer() fyne.WidgetRenderer {
	fill := theme.Color(theme.ColorNameBackground)
	if p.fillColor != nil {
		fill = p.fillColor()
	}
	rect := canvas.NewRectangle(fill)
	rect.StrokeWidth = p.strokeWidth
	// Initial stroke visible in both themes; exact value set in Refresh.
	rect.StrokeColor = color.NRGBA{R: 220, G: 220, B: 220, A: 255}
	rect.CornerRadius = p.cornerRadius
	objs := []fyne.CanvasObject{rect}
	return &themedPanelRenderer{panel: p, rect: rect, objects: objs}
}

type themedPanelRenderer struct {
	panel   *ThemedPanel
	rect    *canvas.Rectangle
	objects []fyne.CanvasObject
}

func (r *themedPanelRenderer) Layout(size fyne.Size) {
	if r.panel != nil && r.panel.inset > 0 {
		inset := r.panel.inset
		r.rect.Move(fyne.NewPos(inset, inset))
		r.rect.Resize(fyne.NewSize(size.Width-2*inset, size.Height-2*inset))
	} else {
		r.rect.Move(fyne.NewPos(0, 0))
		r.rect.Resize(size)
	}
}

func (r *themedPanelRenderer) MinSize() fyne.Size {
	return fyne.NewSize(10, 10)
}

func (r *themedPanelRenderer) Refresh() {
	// Fill follows current theme
	if r.panel.fillColor != nil {
		r.rect.FillColor = r.panel.fillColor()
	} else {
		r.rect.FillColor = theme.Color(theme.ColorNameBackground)
	}
	// Rounded corners using configured radius
	r.rect.CornerRadius = r.panel.cornerRadius
	// Subtle border depends on theme variant
	variant := theme.VariantLight
	if r.panel != nil && r.panel.app != nil {
		variant = r.panel.app.Settings().ThemeVariant()
	}
	if r.panel.strokeWidth <= 0 {
		r.rect.StrokeWidth = 0
		r.rect.StrokeColor = color.Transparent
	} else {
		r.rect.StrokeWidth = r.panel.strokeWidth
		if variant == theme.VariantDark {
			r.rect.StrokeColor = color.NRGBA{R: 70, G: 70, B: 70, A: 160}
		} else {
			r.rect.StrokeColor = color.NRGBA{R: 220, G: 220, B: 220, A: 255}
		}
	}
	canvas.Refresh(r.rect)
}

func (r *themedPanelRenderer) BackgroundColor() color.Color { return color.Transparent }
func (r *themedPanelRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *themedPanelRenderer) Destroy()                     {}

var i18n = map[string]map[string]string{
	"en": {
		"endpoint":            "Endpoint",
		"address_space":       "Address Space",
		"connect":             "Connect",
		"disconnect":          "Disconnect",
		"connecting":          "Connecting...",
		"settings":            "Settings",
		"export":              "Export",
		"add_to_watch":        "Add to Watch",
		"write_value":         "Write Value",
		"remove":              "Remove",
		"write":               "Write",
		"export_dialog":       "Export Address Space",
		"format":              "Format",
		"scope":               "Scope",
		"all":                 "All",
		"folder":              "Folder",
		"folder_nodeid":       "Folder NodeID",
		"recursive":           "Recursive",
		"options":             "Options",
		"folder_nodeid_error": "Please enter a valid Folder NodeID",
		"export_btn":          "Export",
		"cancel_btn":          "Cancel",
		"language":            "Language",
		"lang_en":             "English",
		"lang_zh":             "中文",
		"watch_list":          "Watch List",
		"selected_details":    "Attribute",
		"logs":                "Logs",
		"clear_all":           "Clear All",
		"clear_logs":          "Clear Logs",
		"copy":                "Copy",
		"running_on":          "Running on",
		// Auth mode labels (for localization of radio group)
		"anonymous":          "Anonymous",
		"username":           "Username",
		"certificate":        "Certificate",
		"api_disabled":       "API Disabled",
		"api_server_stopped": "API Server Stopped",
		// Settings dialog
		"connection_settings":     "Connection Settings",
		"save_btn":                "Save",
		"endpoint_url":            "Endpoint URL",
		"discover_endpoints":      "Discover",
		"discovering":             "Discovering...",
		"select_endpoint":         "Select Endpoint",
		"application_uri":         "Application URI",
		"product_uri":             "Product URI",
		"session_timeout_s":       "Session Timeout (s)",
		"connect_timeout_s":       "Connect Timeout (s)",
		"security_policy":         "Security Policy",
		"security_mode":           "Security Mode",
		"authentication":          "Authentication",
		"api_port":                "API Port",
		"enable_api":              "Enable API/Web Server",
		"auto_connect":            "Auto-connect on startup",
		"disable_logs":            "Disable logs",
		"placeholder_app_uri":     "urn:hostname:client",
		"placeholder_product_uri": "urn:your-company:product",
		"placeholder_api_port":    "e.g.,8080",
		"placeholder_timeout_s":   "in seconds",
		"placeholder_username":    "Username",
		"placeholder_password":    "Password",
		"placeholder_cert_file":   "Client certificate file (.der/.crt)",
		"placeholder_key_file":    "Private key file (.key/.pem)",
		"browse":                  "Browse...",
		"auto_generate_cert":      "Auto-generate certificates",
		"generate_cert":           "Generate Certificates",
		"cert_info":               "Certificate Info",
	},
	"zh": {
		"endpoint":            "服务端地址",
		"address_space":       "地址空间",
		"connect":             "连接",
		"disconnect":          "断开",
		"connecting":          "连接中...",
		"settings":            "设置",
		"export":              "导出",
		"add_to_watch":        "加入监视",
		"write_value":         "写入数值",
		"remove":              "移除",
		"write":               "写入",
		"export_dialog":       "导出地址空间",
		"format":              "格式",
		"scope":               "范围",
		"all":                 "全部",
		"folder":              "文件夹",
		"folder_nodeid":       "文件夹NodeID",
		"recursive":           "递归",
		"options":             "选项",
		"folder_nodeid_error": "请输入有效的文件夹NodeID",
		"export_btn":          "导出",
		"cancel_btn":          "取消",
		"language":            "语言",
		"lang_en":             "英文",
		"lang_zh":             "中文",
		"watch_list":          "监视列表",
		"selected_details":    "属性",
		"logs":                "日志",
		"clear_all":           "清空全部",
		"clear_logs":          "清空日志",
		"copy":                "复制",
		"running_on":          "运行在",
		// Auth mode labels
		"anonymous":          "匿名",
		"username":           "用户名",
		"certificate":        "证书",
		"api_disabled":       "API已禁用",
		"api_server_stopped": "API服务已停止",
		// Settings dialog
		"connection_settings":     "连接设置",
		"save_btn":                "保存",
		"endpoint_url":            "服务端地址",
		"discover_endpoints":      "发现端点",
		"discovering":             "正在发现...",
		"select_endpoint":         "选择端点",
		"application_uri":         "应用URI",
		"product_uri":             "产品URI",
		"session_timeout_s":       "会话超时(秒)",
		"connect_timeout_s":       "连接超时(秒)",
		"security_policy":         "安全策略",
		"security_mode":           "安全模式",
		"authentication":          "认证方式",
		"api_port":                "API端口",
		"enable_api":              "启用 API/网页服务",
		"auto_connect":            "启动时自动连接",
		"disable_logs":            "关闭日志",
		"placeholder_app_uri":     "urn:hostname:client",
		"placeholder_product_uri": "urn:your-company:product",
		"placeholder_api_port":    "例如,8080",
		"placeholder_timeout_s":   "单位:秒",
		"placeholder_username":    "用户名",
		"placeholder_password":    "密码",
		"placeholder_cert_file":   "客户端证书文件 (.der/.crt)",
		"placeholder_key_file":    "私钥文件 (.key/.pem)",
		"browse":                  "浏览...",
		"auto_generate_cert":      "自动生成证书",
		"generate_cert":           "生成证书",
		"cert_info":               "证书信息",
	},
}

// applyLanguage updates visible texts according to current ui.config.Language.
// Call this after changing Language to refresh UI labels/buttons.
func (ui *UI) applyLanguage() {
	if ui == nil {
		return
	}
	// Buttons
	if ui.connectBtn != nil {
		if ui.isConnected {
			ui.connectBtn.SetText(ui.t("disconnect"))
		} else {
			// If currently in connecting state (disabled and showing connecting...), keep it.
			if !(ui.connectBtn.Disabled() && ui.connectBtn.Text == ui.t("connecting")) {
				ui.connectBtn.SetText(ui.t("connect"))
			}
		}
		ui.connectBtn.Refresh()
	}
	if ui.configBtn != nil {
		ui.configBtn.SetText(ui.t("settings"))
		ui.configBtn.Refresh()
	}
	if ui.exportBtn != nil {
		ui.exportBtn.SetText(ui.t("export"))
		ui.exportBtn.Refresh()
	}
	if ui.clearAllBtn != nil {
		ui.clearAllBtn.SetText(ui.t("clear_all"))
		ui.clearAllBtn.Refresh()
	}
	if ui.watchBtn != nil {
		ui.watchBtn.SetText(ui.t("add_to_watch"))
		ui.watchBtn.Refresh()
	}
	if ui.removeWatchBtn != nil {
		ui.removeWatchBtn.SetText(ui.t("remove"))
		ui.removeWatchBtn.Refresh()
	}
	if ui.writeWatchBtn != nil {
		ui.writeWatchBtn.SetText(ui.t("write"))
		ui.writeWatchBtn.Refresh()
	}
	if ui.clearLogBtn != nil {
		ui.clearLogBtn.SetText(ui.t("clear_logs"))
		ui.clearLogBtn.Refresh()
	}
	if ui.copyLogBtn != nil {
		ui.copyLogBtn.SetText(ui.t("copy"))
		ui.copyLogBtn.Refresh()
	}

	// Cards / Labels
	if ui.connectionCard != nil {
		ui.connectionCard.SetTitle(ui.t("endpoint"))
		ui.connectionCard.Refresh()
	}
	if ui.addressSpaceCard != nil {
		ui.addressSpaceCard.SetTitle(ui.t("address_space"))
		ui.addressSpaceCard.Refresh()
	}
	if ui.watchCard != nil {
		ui.watchCard.SetTitle(ui.t("watch_list"))
		ui.watchCard.Refresh()
	}
	if ui.detailsTitleLbl != nil {
		ui.detailsTitleLbl.SetText(ui.t("selected_details"))
		ui.detailsTitleLbl.Refresh()
	}
	if ui.logTitleLbl != nil {
		ui.logTitleLbl.SetText(ui.t("logs"))
		ui.logTitleLbl.Refresh()
	}

	// 当语言变化可能影响文本宽度时，更新详情表左列宽度
	if ui.nodeInfoTable != nil {
		ui.updateDetailsColumnWidths()
	}
}

func (ui *UI) t(key string) string {
	lang := "en"
	if ui != nil && ui.config != nil && ui.config.Language != "" {
		lang = ui.config.Language
	}
	if m, ok := i18n[lang]; ok {
		if v, ok2 := m[key]; ok2 {
			return v
		}
	}
	return key
}

// localizeApiStatus maps backend English status strings to localized UI strings
// and normalizes the "Running on" prefix using i18n.
func (ui *UI) localizeApiStatus(s string) string {
	if s == "API Disabled" {
		return ui.t("api_disabled")
	}
	if s == "API Server Stopped" {
		return ui.t("api_server_stopped")
	}
	if strings.HasPrefix(s, "Running on :") {
		suffix := strings.TrimPrefix(s, "Running on :")
		return ui.t("running_on") + ":" + suffix
	}
	if strings.HasPrefix(s, "Running on:") {
		suffix := strings.TrimPrefix(s, "Running on:")
		return ui.t("running_on") + ":" + suffix
	}
	return s
}

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
			<path d="M 135 79 L 180 100.61 L 180 154.39 L 135 176 L 90 154.39 L 90 100.61 Z" fill="#1ba1e2"/>
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

	// Cards to allow retitling on language change
	connectionCard   *widget.Card
	addressSpaceCard *widget.Card
	// Middle/Right panels
	watchCard       *widget.Card
	detailsCard     *widget.Card
	detailsTitleLbl *widget.Label

	// ...
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
	clearAllBtn      *widget.Button
	clearLogBtn      *widget.Button
	copyLogBtn       *widget.Button
	logTitleLbl      *widget.Label

	logText    *widget.RichText
	logScroll  *container.Scroll
	logMutex   sync.Mutex
	logBuilder *strings.Builder

	// Track live connection state for language-aware button text
	isConnected bool
}

func NewUI(c *controller.Controller, apiStatus *string) *UI {
	a := app.NewWithID("com.giantbaby.opcuababy") // Use App ID for storage
	// Only change font on iOS; keep all other visuals from the default theme.
	a.Settings().SetTheme(&fontOnlyTheme{base: theme.DefaultTheme()})
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
			EndpointURL:      "opc.tcp://127.0.0.1:4840",
			SecurityPolicy:   "Auto",
			SecurityMode:     "Auto",
			AuthMode:         "Anonymous",
			ApplicationURI:   "",
			ProductURI:       "",
			SessionTimeout:   30,
			ApiPort:          "8080",
			ApiEnabled:       true,
			ConnectTimeout:   5, // Default 5-second timeout
			Language:         "en",
			AutoGenerateCert: runtime.GOOS == "ios" || runtime.GOOS == "android", // Enable by default on mobile
		},
		apiStatusLabel: widget.NewLabel(*apiStatus),
	}

	ui.loadConfig()

	// Set initial localized API status text
	ui.initWidgets()
	ui.apiStatusLabel.SetText(ui.localizeApiStatus(*apiStatus))
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

	// Periodically update and localize API status label
	go func() {
		for {
			time.Sleep(1 * time.Second)
			fyne.Do(func() {
				ui.apiStatusLabel.SetText(ui.localizeApiStatus(*apiStatus))
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

	// Ensure full cleanup on app close: stop API server, disconnect OPC client, clear state
	w.SetCloseIntercept(func() {
		// Best-effort shutdown before window closes
		ui.controller.Shutdown()
		// proceed to close the window/app
		w.Close()
	})
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

	ui.connectBtn = widget.NewButtonWithIcon(ui.t("connect"), theme.LoginIcon(), ui.onConnectClicked)
	ui.configBtn = widget.NewButtonWithIcon(ui.t("settings"), theme.SettingsIcon(), ui.showConfigDialog)
	ui.exportBtn = widget.NewButtonWithIcon(ui.t("export"), theme.DownloadIcon(), ui.showExportDialog)

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
		//ui.controller.Log(fmt.Sprintf("[blue]Tree OnSelected: %s[-]", string(uid)))
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
		//ui.controller.Log(fmt.Sprintf("[blue]Tree OnUnselected: %s[-]", string(uid)))
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
	// 自适应设置左列宽度，保证能容纳属性名称
	ui.updateDetailsColumnWidths()

	ui.watchTable = widget.NewTable(
		func() (int, int) {
			ui.watchTableMutex.RLock()
			defer ui.watchTableMutex.RUnlock()
			return len(ui.watchRows) + 1, 12
		},
		func() fyne.CanvasObject {
			lbl := widget.NewLabel("")
			rect := canvas.NewRectangle(color.Transparent)
			return container.NewStack(rect, lbl)
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

	ui.watchBtn = widget.NewButtonWithIcon(ui.t("add_to_watch"), theme.ContentAddIcon(), func() {
		if ui.selectedNodeID != "" {
			ui.controller.AddWatch(string(ui.selectedNodeID))
		}
	})

	// 【已修正】: 清理了所有混乱的旧代码和语法错误，只保留正确的实现。
	ui.writeBtn = widget.NewButtonWithIcon(ui.t("write_value"), theme.DocumentCreateIcon(), func() {
		if ui.selectedNodeID == "" {
			return
		}
		nid := string(ui.selectedNodeID)
		go ui.openWriteForNode(nid)
	})

	ui.watchBtn.Disable()
	ui.writeBtn.Disable()

	ui.removeWatchBtn = widget.NewButtonWithIcon(ui.t("remove"), theme.DeleteIcon(), func() {
		if ui.selectedWatchRow < 0 || ui.selectedWatchRow >= len(ui.watchRows) {
			return
		}
		nodeID := ui.watchRows[ui.selectedWatchRow].NodeID
		go ui.controller.RemoveWatch(nodeID)
	})
	ui.selectedWatchRow = -1
	ui.removeWatchBtn.Disable()

	ui.writeWatchBtn = widget.NewButtonWithIcon(ui.t("write"), theme.DocumentCreateIcon(), func() {
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
			// keep internal state in sync so applyLanguage() renders correct button text
			ui.isConnected = connected
			ui.connectBtn.Enable()
			if connected {
				ui.connectBtn.SetText(ui.t("disconnect"))
				ui.connectBtn.SetIcon(theme.LogoutIcon())
				ui.statusIcon.SetResource(theme.ConfirmIcon())
				ui.nodeTree.Root = ui.virtualRoot
				ui.nodeTree.OpenBranch(ui.virtualRoot)
			} else {
				ui.connectBtn.SetText(ui.t("connect"))
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
			// 属性内容可能变化，更新列宽（左列适配名称，右列适配值或占满剩余宽度）
			ui.updateDetailsColumnWidths()

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
	if ui.controller == nil {
		return
	}

	// Check if already connected or connecting
	if ui.isConnected {
		ui.controller.Disconnect()
		return
	}

	// Disable button and show connecting state
	ui.connectBtn.Disable()
	ui.connectBtn.SetText(ui.t("connecting"))
	ui.connectBtn.Refresh()

	go func() {
		// Certificate handling is now done in config.ToOpcuaOptions()
		// No need to call EnsureCertificates here as it's handled automatically

		err := ui.controller.Connect(ui.config)
		fyne.Do(func() {
			ui.connectBtn.Enable()
			if err != nil {
				ui.connectBtn.SetText(ui.t("connect"))
			}
			ui.connectBtn.Refresh()
		})
	}()
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
	appURIEntry.SetPlaceHolder(ui.t("placeholder_app_uri"))
	appURIEntry.SetText(ui.config.ApplicationURI)

	productURIEntry := widget.NewEntry()
	productURIEntry.SetPlaceHolder(ui.t("placeholder_product_uri"))
	productURIEntry.SetText(ui.config.ProductURI)

	sessionTimeoutEntry := widget.NewEntry()
	sessionTimeoutEntry.SetPlaceHolder(ui.t("placeholder_timeout_s"))
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

	// Authentication (User Identity): only Anonymous/Username in UI. Certificate belongs to security channel, not user identity.
	valueToDisplay := map[string]string{
		"Anonymous": ui.t("anonymous"),
		"Username":  ui.t("username"),
	}
	displayToValue := map[string]string{
		ui.t("anonymous"): "Anonymous",
		ui.t("username"):  "Username",
	}
	// Start with both options; may be narrowed by endpoint discovery selection below.
	authOptions := []string{valueToDisplay["Anonymous"], valueToDisplay["Username"]}
	authModeRadio := widget.NewRadioGroup(authOptions, nil)
	if disp, ok := valueToDisplay[ui.config.AuthMode]; ok {
		authModeRadio.SetSelected(disp)
	} else {
		authModeRadio.SetSelected(valueToDisplay["Anonymous"]) // default
	}
	authModeRadio.Horizontal = true

	userEntry := widget.NewEntry()
	userEntry.SetPlaceHolder(ui.t("placeholder_username"))
	userEntry.SetText(ui.config.Username)
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder(ui.t("placeholder_password"))
	passwordEntry.SetText(ui.config.Password)
	// Remove inner labels to align entries with other form fields for maximum width
	userPassContainer := container.NewVBox(userEntry, passwordEntry)

	// Security channel certificate/key (not user identity)
	certFileEntry := widget.NewEntry()
	certFileEntry.SetPlaceHolder(ui.t("placeholder_cert_file"))
	certFileEntry.SetText(ui.config.CertFile)
	certBrowseBtn := widget.NewButton(ui.t("browse"), func() {
		dlg := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()
			uri := reader.URI()
			p := uri.Path()
			if p == "" {
				// On iOS, some providers return non-file URIs. Fallback to URI string.
				certFileEntry.SetText(uri.String())
			} else {
				certFileEntry.SetText(p)
			}
		}, ui.window)
		dlg.SetFilter(storage.NewExtensionFileFilter([]string{".der", ".crt", ".cer"}))
		dlg.Show()
	})
	certRow := container.NewBorder(nil, nil, nil, certBrowseBtn, certFileEntry)

	keyFileEntry := widget.NewEntry()
	keyFileEntry.SetPlaceHolder(ui.t("placeholder_key_file"))
	keyFileEntry.SetText(ui.config.KeyFile)
	keyBrowseBtn := widget.NewButton(ui.t("browse"), func() {
		dlg := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()
			uri := reader.URI()
			p := uri.Path()
			if p == "" {
				keyFileEntry.SetText(uri.String())
			} else {
				keyFileEntry.SetText(p)
			}
		}, ui.window)
		dlg.SetFilter(storage.NewExtensionFileFilter([]string{".key", ".pem"}))
		dlg.Show()
	})
	keyRow := container.NewBorder(nil, nil, nil, keyBrowseBtn, keyFileEntry)

	// Certificate generation button
	generateCertBtn := widget.NewButton(ui.t("generate_cert"), func() {
		// Force-generate new certificates (overwrite existing)
		certPath, keyPath, err := cert.ForceGenerateCertificates()
		if err != nil {
			ui.controller.Log(fmt.Sprintf("[red]Failed to generate certificates: %v[-]", err))
			dialog.ShowError(fmt.Errorf("failed to generate certificates: %v", err), ui.window)
			return
		}

		// Update UI fields and live config so connections use new files immediately
		certFileEntry.SetText(certPath)
		keyFileEntry.SetText(keyPath)
		ui.config.CertFile = certPath
		ui.config.KeyFile = keyPath

		// Optionally show certificate info after generation
		if info, err := cert.GetCertificateInfo(certPath); err == nil {
			dialog.ShowInformation(ui.t("cert_info"), info, ui.window)
		}
	})

	// Only keep the Generate button in the actions row
	certActionsRow := container.NewHBox(generateCertBtn)

	// Declare holder early so updateSecurityFields() can reference it safely
	var credHolder *fyne.Container

	// Manage security-dependent visibility and auth options
	updateSecurityFields := func() {
		mode := modeSelect.Selected
		if mode == "None" {
			// Hide and disable certificate/key rows when not needed
			certRow.Hide()
			keyRow.Hide()
			certActionsRow.Hide()
			certFileEntry.Disable()
			certBrowseBtn.Disable()
			keyFileEntry.Disable()
			keyBrowseBtn.Disable()
			generateCertBtn.Disable()

			// Show authentication selection (Anonymous only) and hide credentials when insecure
			authModeRadio.Show()
			credHolder.Hide()

			// Force Anonymous over insecure channel
			anonDisp := valueToDisplay["Anonymous"]
			if len(authModeRadio.Options) != 1 || authModeRadio.Options[0] != anonDisp {
				authModeRadio.Options = []string{anonDisp}
			}
			if displayToValue[authModeRadio.Selected] != "Anonymous" {
				authModeRadio.SetSelected(anonDisp)
			}
		} else {
			// Security mode is Sign or SignAndEncrypt
			// Show authentication selection and credentials section (content toggled by radio)
			authModeRadio.Show()
			credHolder.Show()
			// Ensure Username option is visible/enabled when secure
			if len(authOptions) == 0 {
				authOptions = []string{valueToDisplay["Anonymous"], valueToDisplay["Username"]}
			} else {
				// Guarantee Username is present per requirement when secure
				hasUser := false
				for _, o := range authOptions {
					if o == valueToDisplay["Username"] {
						hasUser = true
						break
					}
				}
				if !hasUser {
					authOptions = append(authOptions, valueToDisplay["Username"]) // add Username
				}
			}
			// Apply updated options to the radio group (important when switching from None)
			authModeRadio.Options = authOptions
			// Ensure current selection is valid
			valid := false
			for _, opt := range authOptions {
				if opt == authModeRadio.Selected {
					valid = true
					break
				}
			}
			if !valid {
				authModeRadio.SetSelected(authOptions[0])
			}

			// If switching to Sign, prefer Username immediately
			if mode == "Sign" {
				userDisp := valueToDisplay["Username"]
				// Only switch if available
				for _, opt := range authModeRadio.Options {
					if opt == userDisp && authModeRadio.Selected != userDisp {
						authModeRadio.SetSelected(userDisp)
						break
					}
				}
			}

			// Show cert/key for both Sign and SignAndEncrypt
			if mode == "SignAndEncrypt" || mode == "Sign" {
				certRow.Show()
				keyRow.Show()
				certActionsRow.Show()
				certFileEntry.Enable()
				certBrowseBtn.Enable()
				keyFileEntry.Enable()
				keyBrowseBtn.Enable()
				generateCertBtn.Enable()
			} else {
				certRow.Hide()
				keyRow.Hide()
				certActionsRow.Hide()
				certFileEntry.Disable()
				certBrowseBtn.Disable()
				keyFileEntry.Disable()
				keyBrowseBtn.Disable()
				generateCertBtn.Disable()
			}
		}
		// Refresh affected rows
		certRow.Refresh()
		keyRow.Refresh()
		authModeRadio.Refresh()
		credHolder.Refresh()
	}
	policySelect.OnChanged = func(sel string) {
		// If policy becomes None, enforce mode None
		if sel == "None" && modeSelect.Selected != "None" {
			modeSelect.SetSelected("None")
		}
		// If policy becomes specific (not None/Auto) while mode is None, set a secure default mode
		if sel != "None" && sel != "Auto" && policySelect.Selected == "None" {
			policySelect.SetSelected("Basic256Sha256")
		}
		updateSecurityFields()
	}
	modeSelect.OnChanged = func(sel string) {
		// If mode becomes None, enforce policy None
		if sel == "None" && policySelect.Selected != "None" {
			policySelect.SetSelected("None")
		}
		// If mode becomes specific (not None/Auto) while policy is None, set a secure default policy
		if sel != "None" && sel != "Auto" && policySelect.Selected == "None" {
			policySelect.SetSelected("Basic256Sha256")
		}
		updateSecurityFields()
	}
	// Holder that shows either user/pass
	credHolder = container.NewVBox()
	setCred := func() {
		switch displayToValue[authModeRadio.Selected] {
		case "Anonymous":
			credHolder.Objects = []fyne.CanvasObject{}
		case "Username":
			credHolder.Objects = []fyne.CanvasObject{userPassContainer}
		}
		credHolder.Refresh()
	}
	authModeRadio.OnChanged = func(selected string) { setCred() }
	// initialize state once (after credHolder and setCred are ready)
	updateSecurityFields()
	setCred()

	apiPortEntry := widget.NewEntry()
	apiPortEntry.SetPlaceHolder(ui.t("placeholder_api_port"))
	apiPortEntry.SetText(ui.config.ApiPort)

	apiEnabledCheck := widget.NewCheck(ui.t("enable_api"), nil)
	apiEnabledCheck.SetChecked(ui.config.ApiEnabled)

	autoConnectCheck := widget.NewCheck(ui.t("auto_connect"), nil)
	autoConnectCheck.SetChecked(ui.config.AutoConnect)

	disableLogCheck := widget.NewCheck(ui.t("disable_logs"), nil)
	disableLogCheck.SetChecked(ui.config.DisableLog)

	langDisplayToCode := map[string]string{
		"English": "en",
		"中文":      "zh",
	}
	langNames := []string{"English", "中文"}
	selectedLangName := "English"
	if ui.config != nil && ui.config.Language == "zh" {
		selectedLangName = "中文"
	}
	languageSelect := widget.NewSelect(langNames, nil)
	languageSelect.SetSelected(selectedLangName)

	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetPlaceHolder(ui.t("placeholder_timeout_s"))
	timeoutEntry.SetText(fmt.Sprintf("%.1f", ui.config.ConnectTimeout))

	// Discover Endpoints button and logic
	discoverBtn := widget.NewButton(ui.t("discover_endpoints"), func() {
		// Determine timeout from field or fallback
		to := ui.config.ConnectTimeout
		if v, err := strconv.ParseFloat(strings.TrimSpace(timeoutEntry.Text), 64); err == nil && v > 0 {
			to = v
		}
		if to <= 0 {
			to = 10 // default 10s
		}

		// Normalize endpoint input
		addr := normalizeEndpoint(strings.TrimSpace(endpointEntry.Text))
		endpointEntry.SetText(addr)

		prog := dialog.NewProgressInfinite(ui.t("discover_endpoints"), ui.t("discovering"), ui.window)
		prog.Show()

		// Helper mappers
		toPolicy := func(policyURI string) string {
			// Expect suffix after '#', e.g., ...#Basic256Sha256
			if idx := strings.LastIndex(policyURI, "#"); idx >= 0 && idx+1 < len(policyURI) {
				return policyURI[idx+1:]
			}
			// Some servers may return just the short name already
			return policyURI
		}
		toMode := func(m ua.MessageSecurityMode) string {
			switch m {
			case ua.MessageSecurityModeNone:
				return "None"
			case ua.MessageSecurityModeSign:
				return "Sign"
			case ua.MessageSecurityModeSignAndEncrypt:
				return "SignAndEncrypt"
			default:
				return "None"
			}
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(to*float64(time.Second)))
			defer cancel()
			eps, err := opcua.GetEndpoints(ctx, addr)
			fyne.Do(func() { prog.Hide() })
			if err != nil {
				fyne.Do(func() { dialog.ShowError(err, ui.window) })
				return
			}
			if len(eps) == 0 {
				fyne.Do(func() { dialog.ShowInformation(ui.t("discover_endpoints"), "No endpoints returned", ui.window) })
				return
			}

			// Build list view
			type row struct {
				display          string
				url              string
				policy           string
				mode             string
				supportsAnon     bool
				supportsUsername bool
			}
			rows := make([]row, 0, len(eps))
			for _, ep := range eps {
				pol := toPolicy(ep.SecurityPolicyURI)
				md := toMode(ep.SecurityMode)
				// Determine supported user token types (limit to Anonymous/UserName for UI)
				supAnon := false
				supUser := false
				if ep.UserIdentityTokens != nil {
					for _, tok := range ep.UserIdentityTokens {
						switch tok.TokenType {
						case ua.UserTokenTypeAnonymous:
							supAnon = true
						case ua.UserTokenTypeUserName:
							supUser = true
						}
					}
				}
				// Display also mentions supported identities for clarity
				tags := make([]string, 0, 2)
				if supAnon {
					tags = append(tags, valueToDisplay["Anonymous"])
				}
				if supUser {
					tags = append(tags, valueToDisplay["Username"])
				}
				extra := ""
				if len(tags) > 0 {
					extra = " | " + strings.Join(tags, ", ")
				}
				disp := fmt.Sprintf("%s\n%s | %s%s", ep.EndpointURL, pol, md, extra)
				rows = append(rows, row{display: disp, url: ep.EndpointURL, policy: pol, mode: md, supportsAnon: supAnon, supportsUsername: supUser})
			}

			fyne.Do(func() {
				list := widget.NewList(
					func() int { return len(rows) },
					func() fyne.CanvasObject { return widget.NewLabel("") },
					func(i widget.ListItemID, o fyne.CanvasObject) { o.(*widget.Label).SetText(rows[i].display) },
				)
				var picker *dialog.CustomDialog
				list.OnSelected = func(id widget.ListItemID) {
					if id < 0 || id >= len(rows) {
						return
					}
					sel := rows[id]
					endpointEntry.SetText(sel.url)
					// Apply policy/mode if they are among our options
					policySelect.SetSelected(sel.policy)
					modeSelect.SetSelected(sel.mode)
					// Narrow auth options based on selected endpoint
					newOpts := make([]string, 0, 2)
					if sel.supportsAnon {
						newOpts = append(newOpts, valueToDisplay["Anonymous"])
					}
					if sel.supportsUsername {
						newOpts = append(newOpts, valueToDisplay["Username"])
					}
					if len(newOpts) == 0 {
						// fallback: keep both to let user try, default Anonymous
						newOpts = []string{valueToDisplay["Anonymous"], valueToDisplay["Username"]}
					}
					authModeRadio.Options = newOpts
					// Remember current endpoint-allowed options for restoration when security != None
					authOptions = newOpts
					// If current selection not available, choose first
					cur := authModeRadio.Selected
					found := false
					for _, opt := range newOpts {
						if opt == cur {
							found = true
							break
						}
					}
					if !found {
						authModeRadio.SetSelected(newOpts[0])
					}
					setCred()
					// Update cert/key enabled state based on policy/mode after selection
					updateSecurityFields()
					if picker != nil {
						picker.Hide()
					}
				}
				content := container.NewVScroll(list)
				content.SetMinSize(fyne.NewSize(480, 300))
				picker = dialog.NewCustom(ui.t("select_endpoint"), ui.t("cancel_btn"), content, ui.window)
				picker.Show()
			})
		}()
	})

	endpointRow := container.NewBorder(nil, nil, nil, discoverBtn, endpointEntry)

	formItems := []*widget.FormItem{
		widget.NewFormItem(ui.t("endpoint_url"), endpointRow),
		widget.NewFormItem(ui.t("application_uri"), appURIEntry),
		widget.NewFormItem(ui.t("product_uri"), productURIEntry),
		widget.NewFormItem(ui.t("session_timeout_s"), sessionTimeoutEntry),
		widget.NewFormItem(ui.t("connect_timeout_s"), timeoutEntry),
		widget.NewFormItem(ui.t("security_policy"), policySelect),
		widget.NewFormItem(ui.t("security_mode"), modeSelect),
		// Place certificate/key next to security settings
		widget.NewFormItem("", certRow),
		widget.NewFormItem("", keyRow),
		widget.NewFormItem("", certActionsRow),
		widget.NewFormItem(ui.t("authentication"), authModeRadio),
		widget.NewFormItem("", credHolder),
		widget.NewFormItem(ui.t("api_port"), apiPortEntry),
		widget.NewFormItem("", apiEnabledCheck),
		widget.NewFormItem("", disableLogCheck),
		widget.NewFormItem("", autoConnectCheck),
		widget.NewFormItem(ui.t("language"), languageSelect),
	}

	d := dialog.NewForm(ui.t("connection_settings"), ui.t("save_btn"), ui.t("cancel_btn"), formItems, func(ok bool) {
		if ok {
			ui.config.EndpointURL = endpointEntry.Text
			ui.endpointEntry.SetText(endpointEntry.Text)
			ui.config.ApplicationURI = appURIEntry.Text
			ui.config.ProductURI = productURIEntry.Text
			ui.config.SecurityPolicy = policySelect.Selected
			ui.config.SecurityMode = modeSelect.Selected
			ui.config.AuthMode = displayToValue[authModeRadio.Selected]
			ui.config.Username = userEntry.Text
			ui.config.Password = passwordEntry.Text
			ui.config.CertFile = certFileEntry.Text
			ui.config.KeyFile = keyFileEntry.Text
			ui.config.ApiPort = apiPortEntry.Text
			ui.config.ApiEnabled = apiEnabledCheck.Checked
			ui.config.AutoConnect = autoConnectCheck.Checked
			ui.config.DisableLog = disableLogCheck.Checked

			if code, ok := langDisplayToCode[languageSelect.Selected]; ok {
				ui.config.Language = code
			}

			if timeout, err := strconv.ParseFloat(timeoutEntry.Text, 64); err == nil {
				ui.config.ConnectTimeout = timeout
			}
			if sTimeout, err := strconv.ParseUint(sessionTimeoutEntry.Text, 10, 32); err == nil {
				ui.config.SessionTimeout = uint32(sTimeout)
			}

			ui.saveConfig()
			// Immediately apply language updates to all visible UI elements
			ui.applyLanguage()
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
		rect.FillColor = theme.Color(theme.ColorNameFocus)
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
		// Ensure the real OPC UA root (i=84) is browsed when the virtual root is expanded,
		// but only if we are connected.
		if ui.controller.GetClientForExport() != nil && ui.controller.GetClientContext() != nil {
			if !ui.controller.HasBrowseBeenPerformed("i=84") && !ui.controller.IsBrowsing("i=84") {
				go ui.controller.Browse("i=84")
			}
		}
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

	// Only trigger a browse when we have a connected client/context to avoid log spam pre-connect.
	if ui.controller.GetClientForExport() != nil && ui.controller.GetClientContext() != nil {
		if !ui.controller.HasBrowseBeenPerformed(string(uid)) && !ui.controller.IsBrowsing(string(uid)) {
			go ui.controller.Browse(string(uid))
		}
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

	// Highest priority: Check for special cases by NodeId (stable) then by name.
	if r.row.nodeID == r.row.ui.virtualRoot {
		// This is the absolute root of the tree, representing the connection.
		iconResource = rootIconResource
	} else {
		uidStr := string(r.row.nodeID)
		// ObjectsFolder is ns=0;i=85
		if uidStr == "ns=0;i=85" || (strings.HasPrefix(uidStr, "ns=0;") && strings.HasSuffix(uidStr, ";i=85")) {
			iconResource = objectsFolderIconResource
			// Server object is ns=0;i=2253
		} else if uidStr == "ns=0;i=2253" || (strings.HasPrefix(uidStr, "ns=0;") && strings.HasSuffix(uidStr, ";i=2253")) {
			iconResource = serverIconResource
		} else if nodeName == "Objects" {
			// Fallback on name in case some servers map different IDs
			iconResource = objectsFolderIconResource
		} else if nodeName == "Server" {
			// Fallback on name if DisplayName is not localized
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
	addItem := fyne.NewMenuItem(r.ui.t("add_to_watch"), func() {
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

// Implement primary tap to ensure selection works even if the Tree's internal handler is not reached.
// This implements fyne.Tappable
func (r *treeRow) Tapped(ev *fyne.PointEvent) {
	if r == nil || r.ui == nil || r.ui.nodeTree == nil {
		return
	}
	// Select the node in the tree (fires OnSelected)
	r.ui.nodeTree.Select(r.nodeID)
	// For branches, toggle open/close to match our UX
	if r.isBranch {
		r.ui.nodeTree.ToggleBranch(r.nodeID)
	}
	// Lightweight log for diagnostics
	// r.ui.controller.Log(fmt.Sprintf("[blue]Row Tapped: %s[-]", string(r.nodeID)))
}

func (ui *UI) showExportDialog() {
	// Format selection: JSON, CSV, Excel
	fileTypeRadio := widget.NewRadioGroup([]string{"JSON", "CSV", "Excel"}, nil)
	fileTypeRadio.SetSelected("JSON")
	fileTypeRadio.Horizontal = true

	// Scope selection: All or Folder
	scopeRadio := widget.NewRadioGroup([]string{ui.t("all"), ui.t("folder")}, nil)
	scopeRadio.SetSelected(ui.t("all"))
	scopeRadio.Horizontal = true

	// Folder NodeID entry (prefill with current selection if available)
	nodeIDEntry := widget.NewEntry()
	nodeIDEntry.SetPlaceHolder("e.g. i=85 or ns=2;s=MyFolder")
	if ui.selectedNodeID != "" {
		nodeIDEntry.SetText(ui.selectedNodeID)
	}
	nodeIDEntry.Disable()

	// Recursive option (applies to Folder scope)
	recursiveCheck := widget.NewCheck(ui.t("recursive"), nil)
	recursiveCheck.Checked = true
	recursiveCheck.Disable()

	// Enable/disable controls based on scope
	scopeRadio.OnChanged = func(s string) {
		isFolder := s == ui.t("folder")
		if isFolder {
			nodeIDEntry.Enable()
			recursiveCheck.Enable()
		} else {
			nodeIDEntry.Disable()
			recursiveCheck.Disable()
		}
	}

	d := dialog.NewForm(ui.t("export_dialog"), ui.t("export_btn"), ui.t("cancel_btn"),
		[]*widget.FormItem{
			widget.NewFormItem(ui.t("format"), fileTypeRadio),
			widget.NewFormItem(ui.t("scope"), scopeRadio),
			widget.NewFormItem(ui.t("folder_nodeid"), nodeIDEntry),
			widget.NewFormItem(ui.t("options"), recursiveCheck),
		},
		func(ok bool) {
			if !ok {
				return
			}
			format := fileTypeRadio.Selected
			scope := scopeRadio.Selected
			nodeID := strings.TrimSpace(nodeIDEntry.Text)
			recursive := recursiveCheck.Checked

			if scope == ui.t("folder") && nodeID == "" {
				dialog.ShowError(errors.New(ui.t("folder_nodeid_error")), ui.window)
				return
			}
			if scope == ui.t("all") {
				nodeID = ""
			}

			var filter storage.FileFilter
			var extension string
			switch format {
			case "JSON":
				filter = storage.NewExtensionFileFilter([]string{".json"})
				extension = ".json"
			case "CSV":
				filter = storage.NewExtensionFileFilter([]string{".csv"})
				extension = ".csv"
			default: // Excel
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
				// Normalize scope back to internal constants ("All"/"Folder")
				scopeInternal := "All"
				if scope == ui.t("folder") {
					scopeInternal = "Folder"
				}
				go ui.runExport(filePath, format, scopeInternal, nodeID, recursive)

			}, ui.window)
			saveDialog.SetFileName("export" + extension)
			saveDialog.SetFilter(filter)
			saveDialog.Show()
		}, ui.window)
	d.Show()
}

func (ui *UI) runExport(filePath, format, scope, nodeID string, recursive bool) {
	client := ui.controller.GetClientForExport()
	if client == nil {
		fyne.CurrentApp().SendNotification(&fyne.Notification{
			Title:   "Export Aborted",
			Content: "Not connected to an OPC UA server.",
		})
		ui.controller.Log("[yellow]Export aborted: not connected.[-]")
		return
	}

	// Determine root node based on scope
	rootID := "i=84"
	if scope == "Folder" && nodeID != "" {
		rootID = nodeID
	}

	// Notify start
	ui.controller.Log(fmt.Sprintf("Starting export (%s) from %s to %s...", format, rootID, filePath))
	fyne.CurrentApp().SendNotification(&fyne.Notification{
		Title:   "Export Started",
		Content: fmt.Sprintf("Building data from %s. This may take some time...", rootID),
	})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var exportErr error
		exporter := exporter.New(client)
		if scope == "Folder" && !recursive {
			// For now, non-recursive export is not implemented in exporter APIs; fall back to recursive
			ui.controller.Log("[yellow]Non-recursive export not yet supported; exporting recursively.[-]")
		}
		switch format {
		case "JSON":
			exportErr = exporter.ExportToJSON(ctx, rootID, filePath)
		case "CSV":
			exportErr = exporter.ExportToCSV(ctx, rootID, filePath)
		default: // Excel
			exportErr = exporter.ExportToExcel(ctx, rootID, filePath)
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
				Content: "Exported to " + filePath,
			})
			ui.controller.Log(fmt.Sprintf("[green]Successfully exported from %s to %s[-]", rootID, filePath))
		}
	}()
}

func (ui *UI) makeLayout() fyne.CanvasObject {
	// Follow system theme automatically using a themed panel that refreshes on theme change
	newBg := func() fyne.CanvasObject {
		return NewThemedPanel(ui.app)
	}

	// Connection section with subtle gray tint and padding
	endpointWithStatus := container.NewBorder(nil, nil, nil, ui.statusIcon,
		container.NewPadded(ui.endpointEntry)) // Add padding around the entry
	connBg := newBg()

	// Create a padded grid for buttons with even spacing
	buttonGrid := container.NewPadded(
		container.NewGridWithColumns(3,
			container.NewHBox(layout.NewSpacer(), ui.connectBtn, layout.NewSpacer()),
			container.NewHBox(layout.NewSpacer(), ui.configBtn, layout.NewSpacer()),
			container.NewHBox(layout.NewSpacer(), ui.exportBtn, layout.NewSpacer()),
		),
	)

	connContent := container.NewStack(
		connBg,
		container.NewVBox(
			endpointWithStatus,
			buttonGrid, // Use the padded grid
			ui.apiStatusLabel,
		),
	)
	// Use plain container to ensure pure white background on all platforms (avoid Card gray on iOS light theme)
	ui.connectionCard = nil
	leftTop := connContent

	// Address space section with the same subtle gray tint
	addrBg := newBg()
	addrContent := container.NewStack(addrBg, ui.nodeTree)
	ui.addressSpaceCard = nil
	leftBottom := addrContent
	leftPanel := container.NewVSplit(leftTop, leftBottom)
	// 延迟设置分割比例以确保渲染器已准备就绪
	defer func() {
		leftPanel.SetOffset(0.19)
	}()

	ui.clearAllBtn = widget.NewButtonWithIcon(ui.t("clear_all"), theme.ContentClearIcon(), ui.controller.RemoveAllWatches)

	// Create a padded container for watch buttons with even spacing
	watchButtons := container.NewPadded(
		container.NewHBox(
			layout.NewSpacer(),
			ui.watchBtn,
			layout.NewSpacer(),
			ui.removeWatchBtn,
			layout.NewSpacer(),
			ui.clearAllBtn,
			layout.NewSpacer(),
			ui.writeWatchBtn,
			layout.NewSpacer(),
		),
	)

	toolbarBg := NewThemedArea(ui.app, func() color.Color { return theme.Color(theme.ColorNameInputBackground) }, 0, 0)
	// Pad the toolbar so the ThemedPanel's top border remains visible and not overdrawn
	toolbar := container.NewPadded(
		container.NewStack(
			toolbarBg,
			container.NewPadded(watchButtons),
		),
	)

	// Watch list with the same subtle gray tint
	watchScroll := container.NewVScroll(ui.watchTable)
	watchBg := newBg()
	watchContent := container.NewStack(
		watchBg,
		container.NewBorder(toolbar, nil, nil, nil,
			container.NewPadded(watchScroll), // Add padding around the watch list
		),
	)
	// No Card for watch list; keep pure container for white background
	ui.watchCard = nil

	// 创建一个自适应宽度的滚动容器，并设置一个合理的最小高度，避免被压扁
	scroll := container.NewVScroll(ui.nodeInfoTable)
	scroll.SetMinSize(fyne.NewSize(0, 240))

	// Details 区域与日志区域结构对齐：背景 + 顶部标题 + 内边距 + 内容
	detailsBg := newBg()
	ui.detailsTitleLbl = widget.NewLabelWithStyle(ui.t("selected_details"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	detailsHeader := container.NewBorder(
		nil, nil,
		ui.detailsTitleLbl,
		nil,
		layout.NewSpacer(),
	)
	detailsContainer := container.NewStack(
		detailsBg,
		container.NewPadded(
			container.NewBorder(
				detailsHeader,
				nil, nil, nil,
				scroll,
			),
		),
	)
	// No Card for details; keep pure container for white background
	ui.detailsCard = nil

	ui.clearLogBtn = widget.NewButtonWithIcon(ui.t("clear_logs"), theme.ContentClearIcon(), ui.clearLogs)
	ui.copyLogBtn = widget.NewButtonWithIcon(ui.t("copy"), theme.ContentCopyIcon(), ui.copyLogs)
	ui.logTitleLbl = widget.NewLabelWithStyle(ui.t("logs"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// 顶部标题栏（右侧：复制 + 清空），添加内边距和按钮间距
	rightBtns := container.NewHBox(
		layout.NewSpacer(),
		ui.copyLogBtn,
		layout.NewSpacer(),
		ui.clearLogBtn,
		layout.NewSpacer(),
	)
	header := container.NewBorder(
		nil, nil,
		ui.logTitleLbl,
		rightBtns,
		layout.NewSpacer(),
	)
	// Logs with the same subtle gray tint
	logBg := newBg()
	// 日志容器使用相同的布局结构保持对齐
	// 简化日志容器构建
	logContainer := container.NewStack(
		logBg,
		container.NewPadded(
			container.NewBorder(
				header, // 顶部放置标题和按钮
				nil, nil, nil,
				ui.logScroll,
			),
		),
	)

	// 取消使用 Card，直接使用容器以获得纯白背景

	// 右侧上下采用可调分割：上（属性）/ 下（日志）- 使用纯容器，避免 Card 的灰背景
	rightPanel := container.NewVSplit(detailsContainer, logContainer)
	rightPanel.SetOffset(0.35)

	// 创建监视列表和右侧面板的水平分割
	centerRightPanel := container.NewHSplit(watchContent, rightPanel)
	// 设置默认分割比例，避免初次渲染错位
	centerRightPanel.SetOffset(0.6)

	mainLayout := container.NewHSplit(leftPanel, centerRightPanel)
	// 设置主分割比例
	mainLayout.SetOffset(0.3)

	// 顶部品牌栏（iOS 上无窗口标题，这里展示应用名与作者）- 无单独背景
	brandLabel := widget.NewLabelWithStyle(" 😃 Big GiantBaby 🍀", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
	brand := container.NewPadded(
		container.NewHBox(
			layout.NewSpacer(),
			brandLabel,
		),
	)
	// 通过 Padded 容器提供的上下内边距保证足够高度

	// 用 Border 将品牌栏置于顶部
	wrapped := container.NewBorder(brand, nil, nil, nil, mainLayout)
	// Outermost background: themed, borderless, follows system theme
	rootBg := NewThemedBackground(ui.app)
	return container.NewStack(rootBg, wrapped)
}

func (ui *UI) copyLogs() {
	ui.logMutex.Lock()
	text := ui.logBuilder.String()
	ui.logMutex.Unlock()
	// 去掉颜色标记 [color] 和 [-]
	clean := regexp.MustCompile(`\[[a-zA-Z]+\]|\[-\]`).ReplaceAllString(text, "")
	ui.app.Clipboard().SetContent(clean)
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

// updateDetailsColumnWidths 根据属性名称文本宽度自适应设置左列宽度
func (ui *UI) updateDetailsColumnWidths() {
	if ui == nil || ui.nodeInfoTable == nil || len(ui.nodeInfoKeys) == 0 {
		return
	}
	// 1) Left column width by longest key
	var leftW float32
	for _, key := range ui.nodeInfoKeys {
		lbl := widget.NewLabel(key)
		lbl.TextStyle = fyne.TextStyle{Bold: true}
		sz := lbl.MinSize()
		if sz.Width > leftW {
			leftW = sz.Width
		}
	}
	padding := float32(theme.Padding()) * 2
	extra := float32(12)
	leftW = leftW + padding + extra
	ui.nodeInfoTable.SetColumnWidth(0, leftW)

	// 2) Right column width by content or remaining width, whichever is larger
	var maxValW float32
	for _, key := range ui.nodeInfoKeys {
		val := ui.nodeInfoData[key]
		lbl := widget.NewLabel(val)
		// measure unwrapped to estimate full width needed (table may scroll horizontally)
		lbl.Wrapping = fyne.TextWrapOff
		sz := lbl.MinSize()
		if sz.Width > maxValW {
			maxValW = sz.Width
		}
	}
	desiredRight := maxValW + padding + extra
	tableW := ui.nodeInfoTable.Size().Width
	if tableW > 0 {
		remaining := tableW - leftW - padding
		if remaining < desiredRight {
			ui.nodeInfoTable.SetColumnWidth(1, desiredRight)
		} else {
			ui.nodeInfoTable.SetColumnWidth(1, remaining)
		}
	} else {
		ui.nodeInfoTable.SetColumnWidth(1, desiredRight)
	}
}

const configName = "opcuababy_config.json"

func (ui *UI) saveConfig() {
	// 1) Save to Preferences (works on iOS/iPadOS)
	if ui.app != nil {
		if data, err := json.Marshal(ui.config); err == nil {
			ui.app.Preferences().SetString("config_json", string(data))
		} else {
			ui.controller.Log(fmt.Sprintf("Failed to marshal config for preferences: %v", err))
		}
	}

	// 2) On non-iOS platforms, additionally write a JSON file next to the executable for backward compatibility
	if runtime.GOOS == "ios" {
		return
	}
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
	if err := os.WriteFile(configFilePath, data, 0644); err != nil {
		ui.controller.Log(fmt.Sprintf("Failed to write config file: %v", err))
		return
	}
}

func (ui *UI) loadConfig() {
	// 1) Try Preferences first (especially for iOS)
	if ui.app != nil {
		if s := ui.app.Preferences().StringWithFallback("config_json", ""); s != "" {
			if err := json.Unmarshal([]byte(s), ui.config); err != nil {
				ui.controller.Log(fmt.Sprintf("Failed to unmarshal preferences config: %v", err))
			}
			return
		}
	}

	// 2) Fallback: read file next to the executable (desktop platforms)
	exePath, err := os.Executable()
	if err != nil {
		ui.controller.Log(fmt.Sprintf("Failed to get executable path: %v", err))
		ui.saveConfig() // save defaults to preferences (and file if supported)
		return
	}
	exeDir := filepath.Dir(exePath)
	configFilePath := filepath.Join(exeDir, configName)
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		ui.saveConfig()
		return
	}
	if err := json.Unmarshal(data, ui.config); err != nil {
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
