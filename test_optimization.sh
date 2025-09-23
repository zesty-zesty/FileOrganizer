#!/bin/bash

# 测试脚本：创建多个测试文件，然后运行优化后的文件整理工具

# 确保测试目录存在
TEST_DIR="$HOME/Desktop/file_organizer_test"
SOURCE_DIR="$TEST_DIR/source"
TARGET_DIR="$TEST_DIR/target"

# 创建测试目录结构
rm -rf "$TEST_DIR"
mkdir -p "$SOURCE_DIR"
mkdir -p "$TARGET_DIR"

# 创建100个测试文件
for i in {1..100}
do
    # 创建不同类型的文件
    if (( i % 3 == 0 )); then
        touch "$SOURCE_DIR/test_file_${i}.txt"
        echo "This is test file $i" > "$SOURCE_DIR/test_file_${i}.txt"
    elif (( i % 3 == 1 )); then
        touch "$SOURCE_DIR/test_file_${i}.pdf"
        echo "PDF content $i" > "$SOURCE_DIR/test_file_${i}.pdf"
    else
        touch "$SOURCE_DIR/test_file_${i}.jpg"
        echo "JPG content $i" > "$SOURCE_DIR/test_file_${i}.jpg"
    fi

sleep 0.1  # 稍微延迟以确保文件修改时间不同
done

echo "已创建100个测试文件在 $SOURCE_DIR"
echo "目标文件夹: $TARGET_DIR"
echo ""
echo "请运行 ./file_organizer_gui_optimized 进行测试，并验证程序性能是否有提升。"
echo "测试完成后，可以删除 $TEST_DIR 目录。"