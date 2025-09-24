package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Config 配置结构体，对应YAML文件
type Config struct {
	SourceDir        string   `yaml:"source_dir"`
	TargetDir        string   `yaml:"target_dir"`
	FileExtensions   []string `yaml:"file_extensions"`
	FolderDateFormat string   `yaml:"folder_date_format"`
	OrganizeRule     string   `yaml:"organize_rule"`
	ExtensionCase    string   `yaml:"extension_case"` // "uppercase" 或 "lowercase"
}

// OrganizeRule 组织规则类型
type OrganizeRule string

const (
	RuleByDate      OrganizeRule = "date"
	RuleByExtension OrganizeRule = "extension"
)

// FileOrganizer 结构体封装所有功能
type FileOrganizer struct {
	SourceDir        string
	TargetDir        string
	FileExtensions   []string
	FolderDateFormat string
	OrganizeRule     OrganizeRule
	SizeRanges       []string
	ExtensionCase    string // "uppercase" 或 "lowercase"

	// GUI组件
	SourceDirEntry      *widget.Label
	TargetDirEntry      *widget.Entry
	ExtensionsEntry     *widget.Entry
	DateFormatSelect    *widget.Select
	RuleSelect          *widget.Select
	ExtensionCaseSelect *widget.Select
	LogTextLabel        *widget.Label
	Window              fyne.Window

	// 额外的UI组件
	scanFilesBtn           *widget.Button
	selectExtensionsBtn    *widget.Button
	selectDateFormatBtn    *widget.Button
	selectExtensionCaseBtn *widget.Button
	processBtn             *widget.Button

	// 日志相关
	logChan          chan string
	logProcessorDone chan struct{}

	// 配置相关
	lastConfigPath string

	// 存储扫描到的文件信息
	scannedFiles          []string
	scannedFileExtensions map[string]bool
}

// NewFileOrganizer 创建新的文件组织器实例
func NewFileOrganizer() *FileOrganizer {
	fo := &FileOrganizer{
		logChan:               make(chan string, 100),
		logProcessorDone:      make(chan struct{}),
		lastConfigPath:        filepath.Join(os.TempDir(), "file_organizer_last_config.yaml"),
		scannedFileExtensions: make(map[string]bool),
		FolderDateFormat:      "YYYY-MM-DD", // 默认文件夹命名规则
		ExtensionCase:         "lowercase",  // 默认扩展名大小写
	}

	// 启动日志处理器
	fo.startLogProcessor()

	return fo
}

// 保存用户配置
func (fo *FileOrganizer) saveUserConfig() {
	// 使用fyne的Preferences API保存配置
	prefs := fyne.CurrentApp().Preferences()
	prefs.SetString("folder_date_format", fo.FolderDateFormat)
	prefs.SetString("extension_case", fo.ExtensionCase)
}

// 加载用户配置
func (fo *FileOrganizer) loadUserConfig() {
	// 使用fyne的Preferences API加载配置
	prefs := fyne.CurrentApp().Preferences()
	// 只有当配置存在且不为空时才加载
	if format := prefs.StringWithFallback("folder_date_format", ""); format != "" {
		fo.FolderDateFormat = format
	}
	if extCase := prefs.StringWithFallback("extension_case", ""); extCase != "" {
		fo.ExtensionCase = extCase
	}
}

// 安全更新UI的函数
func (fo *FileOrganizer) safeUpdateUI(updateFunc func()) {
	if updateFunc != nil {
		fyne.DoAndWait(func() {
			updateFunc()
		})
	}
}

// 启动日志处理器
func (fo *FileOrganizer) startLogProcessor() {
	go func() {
		var buffer strings.Builder
		const maxBufferSize = 1024 * 5                 // 5KB的缓冲区限制 - 减小缓冲区，提高刷新频率
		ticker := time.NewTicker(5 * time.Millisecond) // 5ms的刷新间隔 - 加快刷新速度
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-fo.logChan:
				if !ok {
					// 通道关闭，刷新剩余的日志
					if buffer.Len() > 0 && fo.LogTextLabel != nil {
						logContent := buffer.String()
						fo.safeUpdateUI(func() {
							if fo.LogTextLabel != nil {
								currentText := fo.LogTextLabel.Text
								fo.LogTextLabel.SetText(currentText + logContent)
							}
						})
					}
					close(fo.logProcessorDone)
					return
				}

				// 检查缓冲区大小，如果过大则立即刷新
				if buffer.Len()+len(msg) > maxBufferSize {
					// 先刷新现有缓冲区
					if buffer.Len() > 0 && fo.LogTextLabel != nil {
						logContent := buffer.String()
						fo.safeUpdateUI(func() {
							if fo.LogTextLabel != nil {
								currentText := fo.LogTextLabel.Text
								fo.LogTextLabel.SetText(currentText + logContent)
							}
						})
					}
					buffer.Reset()
				}
				buffer.WriteString(msg)
			case <-ticker.C:
				if buffer.Len() > 0 && fo.LogTextLabel != nil {
					logContent := buffer.String()
					fo.safeUpdateUI(func() {
						if fo.LogTextLabel != nil {
							currentText := fo.LogTextLabel.Text
							fo.LogTextLabel.SetText(currentText + logContent)
							// 限制日志长度，避免内存占用过大
							const maxLogLength = 1024 * 50 // 50KB
							if len(fo.LogTextLabel.Text) > maxLogLength {
								// 保留最后一部分日志
								fo.LogTextLabel.SetText("[日志过长，已截断前部分]\n" +
									fo.LogTextLabel.Text[len(fo.LogTextLabel.Text)-maxLogLength/2:])
							}
						}
					})
					buffer.Reset()
				}
			}
		}
	}()
}

// 停止日志处理器
func (fo *FileOrganizer) stopLogProcessor() {
	close(fo.logChan)
	<-fo.logProcessorDone
}

// 记录日志到UI
func (fo *FileOrganizer) log(message string) {
	logMsg := time.Now().Format("15:04:05") + " - " + message + "\n"

	select {
	case fo.logChan <- logMsg:
	default:
		fo.safeUpdateUI(func() {
			if fo.LogTextLabel != nil {
				fo.LogTextLabel.SetText(fo.LogTextLabel.Text + logMsg)
			}
		})
	}
}

// 创建GUI界面
func (fo *FileOrganizer) createGUI() {
	// 创建应用，使用唯一ID避免Preferences警告
	myApp := app.NewWithID("com.fileorganizer.app")
	fo.Window = myApp.NewWindow("文件整理工具")
	// 现在应用已经创建，可以加载用户配置了
	fo.loadUserConfig()
	fo.Window.Resize(fyne.NewSize(880, 590))

	// 创建UI组件 - 使用Entry作为源文件夹输入框
	fo.SourceDirEntry = widget.NewLabel("")
	//fo.SourceDirEntry.PlaceHolder = "请选择源文件夹"
	//fo.SourceDirEntry.Disable() // 设置为只读

	// 初始化RuleSelect组件（在使用前创建）
	rules := []string{string(RuleByDate), string(RuleByExtension)}
	fo.RuleSelect = widget.NewSelect(rules, nil)
	fo.RuleSelect.SetSelected(string(RuleByDate))
	fo.RuleSelect.Disable() // 初始时禁用，直到选择了源文件夹

	// 初始化LogTextLabel组件（在使用前创建）
	fo.LogTextLabel = widget.NewLabel("")
	fo.LogTextLabel.Wrapping = fyne.TextWrapWord
	fo.LogTextLabel.Alignment = fyne.TextAlignLeading
	fo.LogTextLabel.TextStyle = fyne.TextStyle{Monospace: true}

	// 创建浏览按钮
	sourceBrowseBtn := widget.NewButtonWithIcon("选择源文件夹", theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
			if err == nil && dir != nil {
				fo.SourceDirEntry.SetText(dir.Path())
				// 在按钮完全初始化后设置回调函数
				fo.RuleSelect.OnChanged = func(value string) {
					fo.scanFiles() // 选择规则后自动扫描文件
				}
				fo.RuleSelect.Enable() // 选择了源文件夹后启用规则选择
				fo.log("已选择源文件夹: " + dir.Path())
				// 选择源文件夹后自动扫描文件
				fo.scanFiles()
			}
		}, fo.Window)
	})

	// 选择文件后缀按钮
	fo.selectExtensionsBtn = widget.NewButton("选择文件后缀", func() {
		fo.showSelectExtensionsDialog()
	})
	fo.selectExtensionsBtn.Disable() // 初始时禁用

	// 选择日期格式按钮
	fo.selectDateFormatBtn = widget.NewButton("选择文件夹命名规则", func() {
		fo.showSelectDateFormatDialog()
	})
	fo.selectDateFormatBtn.Disable() // 初始时禁用

	// 选择扩展名大小写按钮
	fo.selectExtensionCaseBtn = widget.NewButton("选择扩展名大小写", func() {
		fo.showSelectExtensionCaseDialog()
	})
	fo.selectExtensionCaseBtn.Disable() // 初始时禁用

	// 处理按钮
	fo.processBtn = widget.NewButton("开始整理", func() {
		fo.processFilesGUI()
	})
	fo.processBtn.Disable() // 初始时禁用

	// 源文件夹区域
	sourceArea := container.NewHBox(
		widget.NewLabel("源文件夹:"),
		fo.SourceDirEntry,
		layout.NewSpacer(),
		sourceBrowseBtn,
	)

	// 整理规则和文件后缀选择
	ruleSection := container.NewGridWithColumns(4,
		widget.NewLabel("整理规则:"),
		fo.RuleSelect,
		widget.NewLabel("文件后缀:"),
		fo.selectExtensionsBtn,
	)

	// 文件夹命名规则和扩展名大小写
	optionSection := container.NewGridWithColumns(4,
		widget.NewLabel("文件夹命名规则:"),
		fo.selectDateFormatBtn,
		widget.NewLabel("扩展名大小写:"),
		fo.selectExtensionCaseBtn,
	)

	// 日志区域
	logScroll := container.NewScroll(fo.LogTextLabel)
	logScroll.SetMinSize(fyne.NewSize(0, 300))
	logSection := container.NewVBox(
		widget.NewLabel("处理日志:"),
		logScroll,
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewButtonWithIcon("清空日志", theme.DeleteIcon(), func() {
				fo.LogTextLabel.SetText("")
			}),
			widget.NewButtonWithIcon("保存日志", theme.DocumentSaveIcon(), func() {
				if fo.LogTextLabel.Text == "" {
					dialog.ShowInformation("提示", "日志为空，无需保存", fo.Window)
					return
				}
				saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
					if err != nil {
						dialog.ShowError(err, fo.Window)
						return
					}
					if writer == nil {
						return
					}
					defer writer.Close()

					_, err = writer.Write([]byte(fo.LogTextLabel.Text))
					if err != nil {
						dialog.ShowError(err, fo.Window)
						return
					}
					fo.log("日志已保存到: " + writer.URI().Path())
				}, fo.Window)
				saveDialog.SetFileName(fmt.Sprintf("file_organizer_log_%s.txt", time.Now().Format("20060102_150405")))
				saveDialog.Show()
			}),
		),
	)

	// 开始整理按钮区域
	processBtnBox := container.NewMax(fo.processBtn)

	// 主布局
	mainContent := container.NewVBox(
		container.NewPadded(sourceArea), // 添加内边距
		container.NewPadded(ruleSection),
		container.NewPadded(optionSection),
		container.NewPadded(processBtnBox),
		container.NewPadded(logSection),
	)

	fo.Window.SetContent(container.NewScroll(mainContent))
	fo.Window.ShowAndRun()

	// 应用退出时停止日志处理器
	fo.stopLogProcessor()
}

// 扫描文件
func (fo *FileOrganizer) scanFiles() {
	sourceDir := fo.SourceDirEntry.Text
	if sourceDir == "" {
		dialog.ShowError(errors.New("请先指定源文件夹"), fo.Window)
		return
	}

	// 验证源文件夹是否存在
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		dialog.ShowError(fmt.Errorf("源文件夹不存在: %s", sourceDir), fo.Window)
		return
	}

	// 清空之前的扫描结果
	fo.scannedFiles = []string{}
	for k := range fo.scannedFileExtensions {
		delete(fo.scannedFileExtensions, k)
	}

	// 清空日志
	fo.LogTextLabel.SetText("")
	fo.log("开始扫描文件...")
	fo.log(fmt.Sprintf("源文件夹: %s", sourceDir))

	// 在goroutine中扫描文件
	go func() {
		err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				fo.scannedFiles = append(fo.scannedFiles, path)
				fileExt := strings.ToLower(filepath.Ext(path))
				if fileExt != "" {
					fo.scannedFileExtensions[fileExt] = true
				}
			}
			return nil
		})

		fo.safeUpdateUI(func() {
			if err != nil {
				fo.log("扫描出错: " + err.Error())
				return
			}

			fo.log(fmt.Sprintf("扫描完成，共发现 %d 个文件", len(fo.scannedFiles)))
			fo.log(fmt.Sprintf("发现 %d 种文件后缀", len(fo.scannedFileExtensions)))

			// 根据选择的规则显示相应的选项
			rule := OrganizeRule(fo.RuleSelect.Selected)
			if rule == RuleByDate {
				fo.selectExtensionsBtn.Enable()
				fo.selectDateFormatBtn.Enable()
				fo.selectExtensionCaseBtn.Disable()
			} else if rule == RuleByExtension {
				fo.selectExtensionsBtn.Enable()
				fo.selectDateFormatBtn.Disable()
				fo.selectExtensionCaseBtn.Enable()
			}
			// 保存当前规则选择
			fo.saveUserConfig()
		})
	}()
}

// 显示选择文件后缀对话框
func (fo *FileOrganizer) showSelectExtensionsDialog() {
	if len(fo.scannedFileExtensions) == 0 {
		dialog.ShowInformation("提示", "请先扫描文件", fo.Window)
		return
	}

	// 创建复选框列表
	var checkboxes []fyne.CanvasObject
	extensionMap := make(map[string]*widget.Check)

	for ext := range fo.scannedFileExtensions {
		checkbox := widget.NewCheck(ext, nil)
		checkboxes = append(checkboxes, checkbox)
		extensionMap[ext] = checkbox
	}

	// 创建滚动容器
	scroll := container.NewVScroll(container.NewVBox(checkboxes...))
	scroll.SetMinSize(fyne.NewSize(400, 300))

	// 创建对话框
	dialog := dialog.NewCustom("选择文件后缀", "确定", scroll, fo.Window)
	dialog.SetOnClosed(func() {
		// 收集选中的后缀
		var selectedExtensions []string
		for ext, checkbox := range extensionMap {
			if checkbox.Checked {
				selectedExtensions = append(selectedExtensions, ext)
			}
		}

		if len(selectedExtensions) > 0 {
			fo.FileExtensions = selectedExtensions
			fo.log(fmt.Sprintf("已选择 %d 种文件后缀进行处理", len(selectedExtensions)))
			fo.processBtn.Enable() // 选择了后缀后启用处理按钮
		} else {
			fo.log("未选择任何文件后缀")
			fo.processBtn.Disable()
		}
	})

	dialog.Show()
}

// 显示选择日期格式对话框
func (fo *FileOrganizer) showSelectDateFormatDialog() {
	dateFormats := []string{"YYYY-MM-DD", "YYYYMMDD", "YY-MM-DD", "YYMMDD"}
	formatSelect := widget.NewSelect(dateFormats, nil)
	// 使用之前保存的文件夹命名规则
	formatSelect.SetSelected(fo.FolderDateFormat)

	dialog := dialog.NewCustom("选择文件夹命名规则", "确定", formatSelect, fo.Window)
	dialog.SetOnClosed(func() {
		fo.FolderDateFormat = formatSelect.Selected
		fo.log(fmt.Sprintf("已选择文件夹命名规则: %s", fo.FolderDateFormat))
		// 保存用户选择的文件夹命名规则
		fo.saveUserConfig()
	})

	dialog.Show()
}

// 显示选择扩展名大小写对话框
func (fo *FileOrganizer) showSelectExtensionCaseDialog() {
	extensionCases := []string{"uppercase", "lowercase"}
	caseSelect := widget.NewSelect(extensionCases, nil)
	// 使用之前保存的扩展名大小写设置
	caseSelect.SetSelected(fo.ExtensionCase)

	dialog := dialog.NewCustom("选择扩展名大小写", "确定", caseSelect, fo.Window)
	dialog.SetOnClosed(func() {
		fo.ExtensionCase = caseSelect.Selected
		fo.log(fmt.Sprintf("已选择扩展名大小写: %s", fo.ExtensionCase))
		// 保存用户选择的扩展名大小写设置
		fo.saveUserConfig()
	})

	dialog.Show()
}

// 处理文件
func (fo *FileOrganizer) processFilesGUI() {
	sourceDir := fo.SourceDirEntry.Text
	if sourceDir == "" {
		dialog.ShowError(errors.New("请指定源文件夹"), fo.Window)
		return
	}

	if len(fo.FileExtensions) == 0 {
		dialog.ShowError(errors.New("请先选择文件后缀"), fo.Window)
		return
	}

	// 获取目标文件夹
	targetDir := sourceDir

	// 创建配置
	config := Config{
		SourceDir:        sourceDir,
		TargetDir:        targetDir,
		FileExtensions:   fo.FileExtensions,
		FolderDateFormat: fo.FolderDateFormat,
		OrganizeRule:     fo.RuleSelect.Selected,
		ExtensionCase:    fo.ExtensionCase,
	}

	fo.log("开始整理文件...")
	fo.log(fmt.Sprintf("源文件夹: %s", sourceDir))
	fo.log(fmt.Sprintf("整理规则: %s", fo.RuleSelect.Selected))
	fo.log(fmt.Sprintf("处理的文件后缀: %v", fo.FileExtensions))

	// 添加进度指示器
	fo.processBtn.Disable()

	// 在goroutine中处理文件
	go func() {
		err := fo.processFiles(config)
		fo.safeUpdateUI(func() {
			if err != nil {
				fo.log("处理出错: " + err.Error())
				fo.processBtn.Enable() // 出错时重新启用按钮
			} else {
				fo.log("处理完成")
			}
		})
	}()
}

// 检查文件是否为需要处理的类型
func (fo *FileOrganizer) isTargetFile(fileExt string, targetExts []string) bool {
	lowerExt := strings.ToLower(fileExt)
	for _, ext := range targetExts {
		if lowerExt == ext {
			return true
		}
	}
	return false
}

// 获取文件修改日期
func (fo *FileOrganizer) getFileModifyDate(fileInfo os.FileInfo, format string) string {
	switch format {
	case "YYYY-MM-DD":
		return fileInfo.ModTime().Format("2006-01-02")
	case "YYYYMMDD":
		return fileInfo.ModTime().Format("20060102")
	case "YY-MM-DD":
		return fileInfo.ModTime().Format("06-01-02")
	case "YYMMDD":
		return fileInfo.ModTime().Format("060102")
	default:
		return fileInfo.ModTime().Format("2006-01-02")
	}
}

// 移动文件到目标目录
func (fo *FileOrganizer) moveFile(sourcePath, targetDir string) error {
	maxRetries := 3

	// 确保目标目录存在
	err := os.MkdirAll(targetDir, 0755)
	if err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 构建目标文件路径
	fileName := filepath.Base(sourcePath)
	targetPath := filepath.Join(targetDir, fileName)

	// 检查目标文件是否已存在
	if _, err := os.Stat(targetPath); err == nil {
		ext := filepath.Ext(fileName)
		name := fileName[:len(fileName)-len(ext)]
		timestamp := time.Now().Format("20060102_150405") // 更精确的时间戳避免冲突
		targetPath = filepath.Join(targetDir, fmt.Sprintf("%s_%s%s", name, timestamp, ext))
	}

	// 尝试重命名文件
	for i := 0; i < maxRetries; i++ {
		err = os.Rename(sourcePath, targetPath)
		if err == nil {
			return nil
		}
		// 只有在不是跨设备移动时才重试（使用字符串判断替代os.ErrCrossDevice）
		if i < maxRetries-1 && !strings.Contains(err.Error(), "cross-device link") {
			time.Sleep(100 * time.Millisecond)
		} else {
			break // 跨设备移动直接尝试复制
		}
	}

	// 如果重命名失败，尝试复制后删除原文件
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer sourceFile.Close()

	targetFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer func() {
		targetFile.Close()
		// 如果发生错误，删除可能创建的不完整目标文件
		if err != nil && targetPath != "" {
			os.Remove(targetPath)
		}
	}()

	// 设置与源文件相同的权限
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("获取源文件信息失败: %w", err)
	}
	targetFile.Chmod(sourceInfo.Mode())

	// 复制文件内容
	_, err = io.Copy(targetFile, sourceFile)
	if err != nil {
		return fmt.Errorf("复制文件内容失败: %w", err)
	}

	// 同步文件到磁盘，确保数据写入完成
	targetFile.Sync()

	// 复制成功后删除源文件
	err = os.Remove(sourcePath)
	if err != nil {
		// 删除失败时记录警告但不返回错误，因为文件已经成功复制
		fo.log(fmt.Sprintf("警告: 已成功复制文件但无法删除原文件 %s: %v", sourcePath, err))
	}

	return nil
}

// 处理文件夹中的文件
func (fo *FileOrganizer) processFiles(config Config) error {
	// 显示找到的文件总数
	fo.log(fmt.Sprintf("将处理 %d 个文件", len(fo.scannedFiles)))

	// 创建工作池进行并行处理
	fileChan := make(chan string, len(fo.scannedFiles))
	resultChan := make(chan string, len(fo.scannedFiles))
	var wg sync.WaitGroup

	// 基于CPU核心数和文件数量智能调整工作协程数
	cpuCount := runtime.NumCPU()
	numWorkers := cpuCount
	if len(fo.scannedFiles) < 20 {
		numWorkers = 2
	} else if numWorkers > 10 {
		numWorkers = 10 // 限制最大工作协程数，避免过多资源消耗
	}

	fo.log(fmt.Sprintf("将使用 %d 个工作协程进行处理", numWorkers))

	// 启动工作协程
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for filePath := range fileChan {
				// 获取文件信息
				fileInfo, err := os.Stat(filePath)
				if err != nil {
					resultChan <- fmt.Sprintf("[工作协程 %d] 获取文件信息失败 %s: %v", workerID, filePath, err)
					continue
				}

				// 检查文件后缀
				fileExt := filepath.Ext(filePath)
				if !fo.isTargetFile(fileExt, config.FileExtensions) {
					resultChan <- fmt.Sprintf("[工作协程 %d] 跳过不符合后缀的文件: %s", workerID, filePath)
					continue
				}

				// 确定目标文件夹路径
				targetDir := ""
				switch OrganizeRule(config.OrganizeRule) {
				case RuleByDate:
					// 按日期组织
					modifyDate := fo.getFileModifyDate(fileInfo, config.FolderDateFormat)
					targetDir = filepath.Join(config.TargetDir, modifyDate)
				case RuleByExtension:
					// 按文件后缀组织
					tempFileExt := filepath.Ext(filePath) // 使用不同的变量名避免重复定义
					if config.ExtensionCase == "uppercase" {
						tempFileExt = strings.ToUpper(tempFileExt)
					} else {
						tempFileExt = strings.ToLower(tempFileExt)
					}
					targetDir = filepath.Join(config.TargetDir, tempFileExt)
				}

				// 移动文件
				err = fo.moveFile(filePath, targetDir)
				if err != nil {
					resultChan <- fmt.Sprintf("[工作协程 %d] 移动文件失败 %s: %v", workerID, filePath, err)
					continue
				}

				resultChan <- fmt.Sprintf("[工作协程 %d] 已移动: %s -> %s", workerID, filepath.Base(filePath), targetDir)
			}
		}(i + 1) // 传递工作协程ID
	}

	// 分发任务
	for _, filePath := range fo.scannedFiles {
		fileChan <- filePath
	}
	close(fileChan)

	// 等待所有工作协程完成并关闭结果通道
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 处理结果
	fileCount := 0
	processedCount := 0
	updateCounter := 0
	updateThreshold := 10

	for result := range resultChan {
		processedCount++
		updateCounter++
		if strings.HasPrefix(result, "[工作协程") && strings.Contains(result, "已移动") {
			fileCount++
		}
		fo.log(result)

		if updateCounter >= updateThreshold {
			fo.safeUpdateUI(func() {
				fo.Window.Content().Refresh()
			})
			updateCounter = 0
		}
	}

	// 最终UI刷新和总结日志
	finalFileCount := fileCount
	finalProcessedCount := processedCount
	fo.safeUpdateUI(func() {
		fo.Window.Content().Refresh()
		fo.log(fmt.Sprintf("处理完成，共检查了 %d 个文件，移动了 %d 个文件", finalProcessedCount, finalFileCount))
		fo.processBtn.Enable() // 处理完成后重新启用按钮
	})
	return nil
}

func main() {
	// 创建文件组织器实例
	organizer := NewFileOrganizer()

	// 创建并显示GUI
	organizer.createGUI()
}
