#!/bin/bash

#!/bin/zsh
# 测试脚本：创建多个测试文件并测试文件整理工具的性能

# 确保测试目录存在
TEST_DIR="$HOME/Desktop/file_organizer_test"
TARGET_DIR="$TEST_DIR/target"
LOG_FILE="$TEST_DIR/test_log.txt"
APP_PATH="$PWD/file_organizer_gui.go"

# 定义多个源文件夹
SOURCE_DIRS=(
    "$TEST_DIR/source1"
    "$TEST_DIR/source2"
    "$TEST_DIR/source3"
    "$TEST_DIR/source4"
)

# 创建测试目录结构
rm -rf "$TEST_DIR"
for dir in "${SOURCE_DIRS[@]}"; do
    mkdir -p "$dir"
done
mkdir -p "$TARGET_DIR"

# 清理旧的日志文件
echo "测试开始时间: $(date)" > "$LOG_FILE"
echo "=======================================" >> "$LOG_FILE"

# 配置测试参数
NUM_FILES=3000  # 增加文件数量以更好地测试性能
FILE_TYPES=('txt' 'pdf' 'jpg' 'png' 'doc' 'docx' 'xls' 'xlsx')  # 增加文件类型多样性

# 创建测试文件
start_file_creation=$(date +%s.%N)
echo "正在创建 $NUM_FILES 个测试文件..."

echo "测试文件夹数量: ${#SOURCE_DIRS[@]}" >> "$LOG_FILE"

# 使用seq命令确保正确展开变量
i=1
while [[ $i -le $NUM_FILES ]]; do
    # 随机选择文件类型
    # 在Zsh中，数组索引从1开始
    index=$(( (RANDOM % ${#FILE_TYPES[@]} ) + 1 ))
    file_type=${FILE_TYPES[$index]}
    
    # 完全随机选择一个源文件夹来放置文件，不再平均分配
    dir_index=$(( (RANDOM % ${#SOURCE_DIRS[@]} ) + 1 )) # Zsh数组索引从1开始
    
    file_path="${SOURCE_DIRS[$dir_index]}/test_file_${i}.${file_type}"
    
    # 随机决定是否在子文件夹中创建文件 (20%概率)
    use_subfolder=$((RANDOM % 5))
    if [[ $use_subfolder -eq 0 ]]; then
        # 创建一个随机子文件夹名
        subfolder_name="subfolder_$((RANDOM % 10 + 1))"
        mkdir -p "${SOURCE_DIRS[$dir_index]}/$subfolder_name"
        file_path="${SOURCE_DIRS[$dir_index]}/$subfolder_name/test_file_${i}.${file_type}"
    else
        file_path="${SOURCE_DIRS[$dir_index]}/test_file_${i}.${file_type}"
    fi
    
    # 创建文件并写入随机大小的内容
    echo "This is test file $i with random content" > "$file_path"
    
    # 随机决定是否添加更多内容，模拟不同大小的文件 (30%概率)
    add_more_content=$((RANDOM % 10))
    if [[ $add_more_content -lt 3 ]]; then
        # 随机决定添加的行数 (1-30行)
        num_extra_lines=$((RANDOM % 30 + 1))
        j=1
        while [[ $j -le $num_extra_lines ]]; do
            echo "Additional line $j for file $i" >> "$file_path"
            j=$((j+1))
        done
    fi
    
    i=$((i+1))
done

end_file_creation=$(date +%s.%N)
file_creation_time=$(echo "$end_file_creation - $start_file_creation" | bc)
echo "文件创建耗时: $file_creation_time 秒" >> "$LOG_FILE"
echo "已创建 $NUM_FILES 个测试文件在 $SOURCE_DIR"

# 记录源目录初始状态
total_source_files=0
for dir in "${SOURCE_DIRS[@]}"; do
    dir_file_count=$(ls -l "$dir" | grep -v '^total' | wc -l)
    total_source_files=$((total_source_files + dir_file_count))
    echo "${dir##*/} 文件数量: $dir_file_count" >> "$LOG_FILE"
done
echo "总源目录文件数量: $total_source_files" >> "$LOG_FILE"

# 编译应用程序（如果需要）
echo "正在编译应用程序..."
go build -o file_organizer_app "$APP_PATH"

if [[ $? -ne 0 ]]; then
    echo "编译失败，请检查代码错误！"
exit 1
fi

# 启动应用程序进行测试
echo ""
echo "测试说明："
echo "1. 应用程序即将启动，请在界面中选择以下参数进行测试："
echo "   - 源文件夹：请选择所有source文件夹（source1, source2, source3, source4）"
echo "   - 目标文件夹：$TARGET_DIR"
echo "   - 文件组织规则：可选择按文件类型或日期"
echo "   - 其他参数保持默认"
echo "2. 点击'开始整理'按钮开始测试"
echo "3. 测试完成后，请记录完成时间和日志显示情况"
echo ""
echo "测试日志已保存至：$LOG_FILE"
echo "测试完成后，可以运行以下命令查看结果："
echo "   ls -l $TARGET_DIR | grep -v '^total' | wc -l  # 查看目标目录文件数量"
echo "   rm -rf $TEST_DIR  # 清理测试文件"
echo ""

# 启动应用程序
./file_organizer_app &

# 保存应用程序PID
APP_PID=$!
echo "应用程序PID: $APP_PID" >> "$LOG_FILE"

# 等待用户手动完成测试
echo "请按回车键继续..."
read

# 记录测试结束信息
echo "测试结束时间: $(date)" >> "$LOG_FILE"
echo "=======================================" >> "$LOG_FILE"

# 统计目标目录中的文件数量
target_file_count=$(ls -l "$TARGET_DIR" | grep -v '^total' | wc -l)
echo "目标目录文件数量: $target_file_count" >> "$LOG_FILE"

success_rate=$(echo "scale=2; $target_file_count * 100 / $source_file_count" | bc)
echo "文件处理成功率: $success_rate%" >> "$LOG_FILE"

# 显示测试结果总结
echo ""
echo "测试结果总结："
echo "- 源目录初始文件数: $source_file_count"
echo "- 目标目录最终文件数: $target_file_count"
echo "- 文件处理成功率: $success_rate%"
echo "- 完整测试日志: $LOG_FILE"
echo ""
echo "测试完成！"