#!/bin/bash

# 构建macOS应用程序脚本 - 使用标准Go工具链

# 版本号
VERSION="v1.2.0"

# 应用名称
APP_NAME="FileOrganizer"

# 构建目录
BUILD_DIR="build"

# 输出目录
OUTPUT_DIR="Releases/$VERSION"

# 清理之前的构建文件
rm -rf $BUILD_DIR $OUTPUT_DIR

# 创建构建目录
mkdir -p $BUILD_DIR
mkdir -p $OUTPUT_DIR

# 使用标准Go工具链构建
echo "正在使用标准Go工具链构建macOS应用程序..."

# 使用标准Go工具链构建
GOOS=darwin GOARCH=arm64 go build -o $BUILD_DIR/$APP_NAME

# 创建.app包结构
mkdir -p $APP_NAME.app/Contents/MacOS
mkdir -p $APP_NAME.app/Contents/Resources
mkdir -p $APP_NAME.app/Contents/Frameworks

# 复制可执行文件
cp $BUILD_DIR/$APP_NAME $APP_NAME.app/Contents/MacOS/

# 复制图标
if [ -f icon.icns ]; then
    cp icon.icns $APP_NAME.app/Contents/Resources/
fi

# 创建Info.plist文件
cat > $APP_NAME.app/Contents/Info.plist << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>English</string>
    <key>CFBundleDisplayName</key>
    <string>$APP_NAME</string>
    <key>CFBundleExecutable</key>
    <string>$APP_NAME</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundleIdentifier</key>
    <string>com.fileorganizer.app</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>$VERSION</string>
    <key>CFBundleVersion</key>
    <string>1.1.0</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>NSHumanReadableCopyright</key>
    <string>© 2025 Imagingvista. All rights reserved.</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

# 移动应用程序到输出目录
if [ -d "$APP_NAME.app" ]; then
    mv "$APP_NAME.app" "$OUTPUT_DIR/${APP_NAME}_Darwin_arm64_$VERSION.app"
    echo "应用程序已打包完成，输出路径：$OUTPUT_DIR/${APP_NAME}_Darwin_arm64_$VERSION.app"
else
    echo "构建失败，未找到生成的.app文件"
    exit 1
fi

# 清理构建目录
rm -rf $BUILD_DIR

# 授予执行权限
chmod +x "$OUTPUT_DIR/${APP_NAME}_Darwin_arm64_$VERSION.app/Contents/MacOS/$APP_NAME"

# 完成提示
echo "macOS应用程序打包成功！"
echo "版本: $VERSION"
echo "架构: arm64"
echo "输出路径: $OUTPUT_DIR/${APP_NAME}_Darwin_arm64_$VERSION.app"