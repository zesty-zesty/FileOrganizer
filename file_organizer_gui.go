package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"gopkg.in/yaml.v3"
)

// Config 配置结构体，对应YAML文件
type Config struct {
	SourceDir        string   `yaml:"source_dir"`
	TargetDir        string   `yaml:"target_dir"`
	FileExtensions   []string `yaml:"file_extensions"`
	FolderDateFormat string   `yaml:"folder_date_format"`
	OrganizeRule     string   `yaml:"organize_rule"`
	SizeRanges       []string `yaml:"size_ranges"`
}

// OrganizeRule 组织规则类型
type OrganizeRule string

const (
	RuleByDate      OrganizeRule = "date"
	RuleBySize      OrganizeRule = "size"
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

	// GUI组件
	SourceDirEntry   *widget.Entry
	TargetDirEntry   *widget.Entry
	ExtensionsEntry  *widget.Entry
	DateFormatSelect *widget.Select
	RuleSelect       *widget.Select
	LogTextLabel     *widget.Label // 使用Label替代Entry以解决文字显示淡的问题
	Window           fyne.Window

	// 日志相关
	logChan          chan string
	logProcessorDone chan struct{}

	// 配置相关
	lastConfigPath string
}

// NewFileOrganizer 创建新的文件组织器实例
func NewFileOrganizer() *FileOrganizer {
	fo := &FileOrganizer{
		logChan:          make(chan string, 100),
		logProcessorDone: make(chan struct{}),
		lastConfigPath:   filepath.Join(os.TempDir(), "file_organizer_last_config.yaml"),
	}

	// 启动日志处理器
	fo.startLogProcessor()

	return fo
}

// 安全更新UI的函数
func (fo *FileOrganizer) safeUpdateUI(updateFunc func()) {
	// 在Fyne v2中，我们使用fyne.DoAndWait确保UI更新在主线程上执行
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
		ticker := time.NewTicker(100 * time.Millisecond) // 更频繁的更新，但每次更新较小批量
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-fo.logChan:
				if !ok {
					// 处理剩余日志
					if buffer.Len() > 0 && fo.LogTextLabel != nil {
						// 保存要处理的日志内容
						logContent := buffer.String()
						// 确保UI操作在主线程执行
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
				buffer.WriteString(msg)
			case <-ticker.C:
				// 只在有内容且UI组件已初始化时更新
				if buffer.Len() > 0 && fo.LogTextLabel != nil {
					// 保存要处理的日志内容
					logContent := buffer.String()
					// 确保UI操作在主线程执行
					fo.safeUpdateUI(func() {
						if fo.LogTextLabel != nil {
							currentText := fo.LogTextLabel.Text
							fo.LogTextLabel.SetText(currentText + logContent)
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

	// 使用通道进行日志消息缓冲
	select {
	case fo.logChan <- logMsg:
		// 消息已发送到通道
	default:
		// 通道已满，直接写入（避免阻塞）
		// 使用安全的UI更新函数
		fo.safeUpdateUI(func() {
			if fo.LogTextLabel != nil {
				fo.LogTextLabel.SetText(fo.LogTextLabel.Text + logMsg)
			}
		})
	}
}

// 创建GUI界面
func (fo *FileOrganizer) createGUI() {
	// 创建应用
	myApp := app.New()
	// 创建窗口并设置较大的初始尺寸，确保日志区域有足够高度
	fo.Window = myApp.NewWindow("文件整理工具")
	fo.Window.Resize(fyne.NewSize(800, 700))
	fo.Window.SetFixedSize(true)

	// 创建UI组件
	fo.SourceDirEntry = widget.NewEntry()
	fo.TargetDirEntry = widget.NewEntry()

	// 优化文件后缀输入框
	fo.ExtensionsEntry = widget.NewEntry()
	fo.ExtensionsEntry.SetPlaceHolder("例如: .txt,.pdf,.jpg,.png,.docx (用逗号分隔)")

	dateFormats := []string{"YYYY-MM-DD", "YYYYMMDD", "YY-MM-DD", "YYMMDD"}
	fo.DateFormatSelect = widget.NewSelect(dateFormats, nil)
	fo.DateFormatSelect.SetSelected("YYYY-MM-DD")

	// 添加组织规则选择
	rules := []string{string(RuleByDate), string(RuleBySize), string(RuleByExtension)}
	fo.RuleSelect = widget.NewSelect(rules, nil)
	fo.RuleSelect.SetSelected(string(RuleByDate))

	// 日志区域设置 - 使用Label组件替代Entry组件解决文字显示淡的问题
	// 关键改进：使用Label组件代替Entry组件，这样文本会以正常亮度显示
	fo.LogTextLabel = widget.NewLabel("")
	fo.LogTextLabel.Wrapping = fyne.TextWrapWord
	fo.LogTextLabel.Alignment = fyne.TextAlignLeading
	// 设置字体大小以提高可读性
	fo.LogTextLabel.TextStyle = fyne.TextStyle{Monospace: true} // 使用等宽字体更适合显示日志

	// 创建浏览按钮
	sourceBrowseBtn := widget.NewButton("浏览...", func() {
		dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
			if err == nil && dir != nil {
				fo.SourceDirEntry.SetText(dir.Path())
			}
		}, fo.Window)
	})

	targetBrowseBtn := widget.NewButton("浏览...", func() {
		dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
			if err == nil && dir != nil {
				fo.TargetDirEntry.SetText(dir.Path())
			}
		}, fo.Window)
	})

	// 处理按钮
	processBtn := widget.NewButton("开始整理", func() {
		fo.processFilesGUI()
	})

	// 保存配置按钮
	saveConfigBtn := widget.NewButton("保存配置", func() {
		fo.saveConfig()
	})

	// 加载配置按钮
	loadConfigBtn := widget.NewButton("加载配置", func() {
		fo.loadConfigGUI()
	})

	// 使用Grid布局，提供更灵活的控件尺寸控制
	form := container.NewVBox(
		container.NewGridWithColumns(3,
			widget.NewLabel("源文件夹:"),
			fo.SourceDirEntry,
			sourceBrowseBtn,
		),
		container.NewGridWithColumns(3,
			widget.NewLabel("目标文件夹:"),
			fo.TargetDirEntry,
			targetBrowseBtn,
		),
		container.NewGridWithColumns(3,
			widget.NewLabel("文件后缀:"),
			fo.ExtensionsEntry,
			widget.NewLabel(""),
		),
		container.NewGridWithColumns(3,
			widget.NewLabel("文件夹格式:"),
			fo.DateFormatSelect,
			widget.NewLabel(""),
		),
		container.NewGridWithColumns(3,
			widget.NewLabel("整理规则:"),
			fo.RuleSelect,
			widget.NewLabel(""),
		),
		container.NewGridWithColumns(2,
			saveConfigBtn,
			loadConfigBtn,
		),
		processBtn,
		widget.NewLabel("处理日志:"),
		// 创建日志区域的滚动容器，并设置固定高度
		container.NewVBox(
			// 创建滚动容器并设置固定高度为300px，确保日志区域有足够的显示空间
			func() *container.Scroll {
				scroll := container.NewScroll(fo.LogTextLabel)
				scroll.SetMinSize(fyne.NewSize(0, 300))
				return scroll
			}(),
			// 添加日志控制按钮
			container.NewGridWithColumns(2,
				widget.NewButton("清空日志", func() {
					fo.LogTextLabel.SetText("")
				}),
				widget.NewButton("保存日志", func() {
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
		),
	)

	fo.Window.SetContent(form)

	// 尝试加载上次的配置
	fo.loadLastConfig()

	fo.Window.ShowAndRun()

	// 应用退出时停止日志处理器
	fo.stopLogProcessor()
}

// 保存配置
func (fo *FileOrganizer) saveConfig() {
	config := Config{
		SourceDir:        fo.SourceDirEntry.Text,
		TargetDir:        fo.TargetDirEntry.Text,
		FileExtensions:   strings.Split(fo.ExtensionsEntry.Text, ","),
		FolderDateFormat: fo.DateFormatSelect.Selected,
		OrganizeRule:     fo.RuleSelect.Selected,
	}

	data, err := yaml.Marshal(&config)
	if err != nil {
		dialog.ShowError(err, fo.Window)
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

		_, err = writer.Write(data)
		if err != nil {
			dialog.ShowError(err, fo.Window)
			return
		}
		fo.log("配置已保存到: " + writer.URI().Path())
	}, fo.Window)
	saveDialog.SetFileName("config.yaml")
	saveDialog.Show()

	// 自动保存到临时文件
	fo.autoSaveConfig(config)
}

// 加载配置
func (fo *FileOrganizer) loadConfigGUI() {
	openDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, fo.Window)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			dialog.ShowError(err, fo.Window)
			return
		}

		var config Config
		err = yaml.Unmarshal(data, &config)
		if err != nil {
			dialog.ShowError(err, fo.Window)
			return
		}

		fo.SourceDirEntry.SetText(config.SourceDir)
		fo.TargetDirEntry.SetText(config.TargetDir)
		fo.ExtensionsEntry.SetText(strings.Join(config.FileExtensions, ","))
		fo.DateFormatSelect.SetSelected(config.FolderDateFormat)
		if config.OrganizeRule != "" {
			fo.RuleSelect.SetSelected(config.OrganizeRule)
		}

		fo.log("已加载配置: " + reader.URI().Path())
	}, fo.Window)
	openDialog.SetFilter(storage.NewExtensionFileFilter([]string{"yaml", "yml"}))
	openDialog.Show()
}

// 自动保存上次使用的配置
func (fo *FileOrganizer) autoSaveConfig(config Config) {
	data, err := yaml.Marshal(&config)
	if err != nil {
		return
	}
	os.WriteFile(fo.lastConfigPath, data, 0644)
}

// 加载上次使用的配置
func (fo *FileOrganizer) loadLastConfig() {
	data, err := os.ReadFile(fo.lastConfigPath)
	if err != nil {
		return
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return
	}

	fo.SourceDirEntry.SetText(config.SourceDir)
	fo.TargetDirEntry.SetText(config.TargetDir)
	fo.ExtensionsEntry.SetText(strings.Join(config.FileExtensions, ","))
	fo.DateFormatSelect.SetSelected(config.FolderDateFormat)
	if config.OrganizeRule != "" {
		fo.RuleSelect.SetSelected(config.OrganizeRule)
	}
}

// 处理文件（GUI版本）
func (fo *FileOrganizer) processFilesGUI() {
	// 验证输入
	sourceDir := fo.SourceDirEntry.Text
	if sourceDir == "" {
		dialog.ShowError(errors.New("请指定源文件夹"), fo.Window)
		return
	}

	// 验证源文件夹是否存在
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		dialog.ShowError(fmt.Errorf("源文件夹不存在: %s", sourceDir), fo.Window)
		return
	}

	targetDir := fo.TargetDirEntry.Text
	if targetDir == "" {
		dialog.ShowError(errors.New("请指定目标文件夹"), fo.Window)
		return
	}

	extensionsText := fo.ExtensionsEntry.Text
	if extensionsText == "" {
		dialog.ShowError(errors.New("请指定文件后缀"), fo.Window)
		return
	}

	// 解析并清理文件后缀（处理空格和空值）
	extensions := strings.Split(extensionsText, ",")
	cleanExtensions := []string{}
	for _, ext := range extensions {
		trimmed := strings.TrimSpace(ext)
		if trimmed != "" {
			// 确保后缀以点开头
			if !strings.HasPrefix(trimmed, ".") {
				trimmed = "." + trimmed
			}
			cleanExtensions = append(cleanExtensions, strings.ToLower(trimmed))
		}
	}

	if len(cleanExtensions) == 0 {
		dialog.ShowError(errors.New("未找到有效的文件后缀，请检查输入"), fo.Window)
		return
	}

	// 创建配置
	config := Config{
		SourceDir:        sourceDir,
		TargetDir:        targetDir,
		FileExtensions:   cleanExtensions,
		FolderDateFormat: fo.DateFormatSelect.Selected,
		OrganizeRule:     fo.RuleSelect.Selected,
	}

	// 自动保存配置
	fo.autoSaveConfig(config)

	// 清空日志
	fo.LogTextLabel.SetText("")
	fo.log("开始处理文件...")
	fo.log(fmt.Sprintf("源文件夹: %s", sourceDir))
	fo.log(fmt.Sprintf("目标文件夹: %s", targetDir))
	fo.log(fmt.Sprintf("处理的文件后缀: %v", cleanExtensions))
	fo.log(fmt.Sprintf("组织规则: %s", fo.RuleSelect.Selected))

	// 刷新界面以显示处理开始的日志
	fo.Window.Content().Refresh()

	// 在goroutine中处理文件，避免UI冻结
	go func() {
		err := fo.processFiles(config)
		// 使用安全的UI更新函数
		fo.safeUpdateUI(func() {
			if err != nil {
				fo.log("处理出错: " + err.Error())
			}
			fo.log("处理完成")

			// 处理完成后刷新界面
			fo.Window.Content().Refresh()
		})
	}()
}

// 检查文件是否为需要处理的类型（忽略大小写）
func (fo *FileOrganizer) isTargetFile(fileExt string, targetExts []string) bool {
	lowerExt := strings.ToLower(fileExt)
	for _, ext := range targetExts {
		if lowerExt == ext {
			return true
		}
	}
	return false
}

// 获取文件修改日期（根据配置的格式）
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

// 获取文件大小分类
func (fo *FileOrganizer) getFileSizeCategory(fileSize int64) string {
	// 简化版：使用预定义的大小分类
	if fileSize < 1024*1024 { // < 1MB
		return "Small (<1MB)"
	} else if fileSize < 10*1024*1024 { // < 10MB
		return "Medium (1MB-10MB)"
	} else {
		return "Large (>10MB)"
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
		// 文件已存在，添加时间戳避免覆盖
		ext := filepath.Ext(fileName)
		name := fileName[:len(fileName)-len(ext)]
		timestamp := time.Now().Format("150405")
		targetPath = filepath.Join(targetDir, fmt.Sprintf("%s_%s%s", name, timestamp, ext))
	}

	// 尝试重命名文件（可能跨文件系统时会失败）
	for i := 0; i < maxRetries; i++ {
		err = os.Rename(sourcePath, targetPath)
		if err == nil {
			return nil
		}
		if i < maxRetries-1 {
			time.Sleep(100 * time.Millisecond) // 短暂延迟后重试
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
	defer targetFile.Close()

	_, err = io.Copy(targetFile, sourceFile)
	if err != nil {
		return fmt.Errorf("复制文件内容失败: %w", err)
	}

	// 复制成功后删除源文件
	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("删除源文件失败: %w", err)
	}

	return nil
}

// 处理文件夹中的文件
func (fo *FileOrganizer) processFiles(config Config) error {
	// 读取源文件夹
	entries, err := os.ReadDir(config.SourceDir)
	if err != nil {
		return fmt.Errorf("读取源文件夹失败: %w", err)
	}

	// 显示找到的文件总数
	fo.log(fmt.Sprintf("在源文件夹中找到 %d 个条目", len(entries)))

	// 过滤出文件（非目录）
	var files []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry)
		}
	}

	// 创建工作池进行并行处理 - 调整缓冲区大小
	fileChan := make(chan os.DirEntry, len(files))
	resultChan := make(chan string, len(files))
	var wg sync.WaitGroup

	// 根据文件数量调整工作协程数，避免过多协程
	numWorkers := 5
	if len(files) < 20 {
		numWorkers = 2 // 少量文件时减少协程数
	}

	// 启动工作协程
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range fileChan {
				// 获取文件信息
				fileInfo, err := entry.Info()
				if err != nil {
					resultChan <- fmt.Sprintf("获取文件信息失败 %s: %v", entry.Name(), err)
					continue
				}

				// 检查文件后缀（忽略大小写）
				fileExt := filepath.Ext(entry.Name())
				if !fo.isTargetFile(fileExt, config.FileExtensions) {
					resultChan <- fmt.Sprintf("跳过不符合后缀的文件: %s (后缀: %s)", entry.Name(), fileExt)
					continue
				}

				// 确定目标文件夹路径
				targetDir := ""
				switch OrganizeRule(config.OrganizeRule) {
				case RuleByDate:
					// 按日期组织
					modifyDate := fo.getFileModifyDate(fileInfo, config.FolderDateFormat)
					targetDir = filepath.Join(config.TargetDir, modifyDate)
				case RuleBySize:
					// 按文件大小组织
					sizeCategory := fo.getFileSizeCategory(fileInfo.Size())
					targetDir = filepath.Join(config.TargetDir, sizeCategory)
				case RuleByExtension:
					// 按文件后缀组织
					targetDir = filepath.Join(config.TargetDir, strings.ToLower(fileExt))
				default:
					// 默认按日期组织
					modifyDate := fo.getFileModifyDate(fileInfo, config.FolderDateFormat)
					targetDir = filepath.Join(config.TargetDir, modifyDate)
				}

				// 移动文件
				sourcePath := filepath.Join(config.SourceDir, entry.Name())
				err = fo.moveFile(sourcePath, targetDir)
				if err != nil {
					resultChan <- fmt.Sprintf("移动文件失败 %s: %v", entry.Name(), err)
					continue
				}

				resultChan <- fmt.Sprintf("已移动: %s -> %s", entry.Name(), targetDir)
			}
		}()
	}

	// 分发任务
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	// 等待所有工作协程完成并关闭结果通道
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 处理结果并更新进度 - 优化日志输出频率
	fileCount := 0
	processedCount := 0
	updateCounter := 0
	updateThreshold := 10 // 每处理10个文件才输出一次进度，避免过多UI更新

	for result := range resultChan {
		processedCount++
		updateCounter++
		// 检查结果是否为成功移动的消息
		if strings.HasPrefix(result, "已移动:") {
			fileCount++
		}
		fo.log(result)

		// 定期刷新UI，但不过于频繁
		if updateCounter >= updateThreshold {
			// 使用安全的UI更新函数
			fo.safeUpdateUI(func() {
				fo.Window.Content().Refresh()
			})
			updateCounter = 0
		}
	}

	// 使用安全的UI更新函数进行最终UI刷新和总结日志
	finalFileCount := fileCount
	finalProcessedCount := processedCount
	fo.safeUpdateUI(func() {
		fo.Window.Content().Refresh()
		fo.log(fmt.Sprintf("处理完成，共检查了 %d 个文件，移动了 %d 个文件", finalProcessedCount, finalFileCount))
	})
	return nil
}

func main() {
	// 创建文件组织器实例
	organizer := NewFileOrganizer()

	// 创建并显示GUI
	organizer.createGUI()
}
