cmake_minimum_required(VERSION 3.18)
project(aria_ebpf_progs LANGUAGES C ASM)

# 设置必要的编译器特性
set(CMAKE_C_STANDARD 99)
set(CMAKE_C_STANDARD_REQUIRED ON)

# 设置构建类型
if(NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Release)
endif()

# 查找必要的工具
find_program(CLANG clang REQUIRED)
find_program(LLC llc REQUIRED)
find_program(LLVM_STRIP llvm-strip REQUIRED)

if(NOT CLANG OR NOT LLC OR NOT LLVM_STRIP)
    message(FATAL_ERROR "Missing required tools: clang, llc, llvm-strip")
endif()

# 获取内核版本
execute_process(
    COMMAND uname -r
    OUTPUT_VARIABLE KERNEL_VERSION
    OUTPUT_STRIP_TRAILING_WHITESPACE
)

# 设置内核头文件路径
set(KERNEL_HEADERS_PATH "/usr/src/linux-headers-$(shell uname -r)")

# 检查标准内核头文件路径
if(NOT EXISTS "${KERNEL_HEADERS_PATH}")
    set(KERNEL_HEADERS_PATH "/lib/modules/$(shell uname -r)/build")
endif()

# 检查是否仍然不存在，如果是则使用通用替代方案
if(NOT EXISTS "${KERNEL_HEADERS_PATH}")
    # 尝试使用 /usr/include 作为基本路径
    set(KERNEL_HEADERS_PATH "/usr/include")
endif()

# 设置包含目录
set(COMMON_INCLUDES
    ${CMAKE_CURRENT_SOURCE_DIR}/include
    ${KERNEL_HEADERS_PATH}
    /usr/src/linux-headers-6.8.0-100/include/uapi
    /usr/include/asm-generic
    ${KERNEL_HEADERS_PATH}/linux
    ${KERNEL_HEADERS_PATH}/asm
    ${KERNEL_HEADERS_PATH}/uapi
    ${KERNEL_HEADERS_PATH}/generated/uapi
)

# 定义编译标志
set(EBPF_CFLAGS
    -g
    -O2
    -target bpf
    -D__TARGET_ARCH_x86
    -Wno-unused-value
    -Wno-pointer-sign
    -Wno-compare-distinct-pointer-types
    -Wunused
    -Wall
    -Wextra
)

# 创建一个函数来编译 eBPF 对象文件
function(compile_bpf_obj target_name src_file)
    set(output_file ${CMAKE_CURRENT_BINARY_DIR}/${target_name}.o)

    add_custom_command(
        OUTPUT ${output_file}
        COMMAND ${CLANG} ${EBPF_CFLAGS}
        -I${CMAKE_CURRENT_SOURCE_DIR}/include
        -I/usr/src/linux-headers-6.8.0-100/include/uapi
        -I/usr/include/asm-generic
        -I${KERNEL_HEADERS_PATH}
        -I${KERNEL_HEADERS_PATH}/linux
        -I${KERNEL_HEADERS_PATH}/asm
        -I${KERNEL_HEADERS_PATH}/uapi
        -I${KERNEL_HEADERS_PATH}/generated/uapi
        -c ${CMAKE_CURRENT_SOURCE_DIR}/progs/${src_file}
        -o ${output_file}
        DEPENDS ${CMAKE_CURRENT_SOURCE_DIR}/progs/${src_file}
        COMMENT "Compiling eBPF program ${target_name}"
        VERBATIM
    )

    add_custom_target(${target_name}_obj ALL
        DEPENDS ${output_file}
    )
endfunction()

# 编译 XDP 程序
compile_bpf_obj(xdp_acl_filter xdp_acl.c)

# 编译 TC 程序
compile_bpf_obj(tc_qos_filter tc_qos.c)

# 安装目标
install(
    FILES
        ${CMAKE_CURRENT_BINARY_DIR}/xdp_acl_filter.o
        ${CMAKE_CURRENT_BINARY_DIR}/tc_qos_filter.o
    DESTINATION lib/aria/ebpf/
)

# 自定义目标：验证 eBPF 程序
add_custom_target(check-ebpf
    COMMAND ${CMAKE_COMMAND} -E echo "Checking eBPF programs..."
    COMMAND test -f ${CMAKE_CURRENT_BINARY_DIR}/xdp_acl_filter.o && echo "✓ xdp_acl_filter.o exists"
    COMMAND test -f ${CMAKE_CURRENT_BINARY_DIR}/tc_qos_filter.o && echo "✓ tc_qos_filter.o exists"
    WORKING_DIRECTORY ${CMAKE_CURRENT_BINARY_DIR}
)

# 打印有用的信息
message(STATUS "eBPF build configuration:")
message(STATUS "  Kernel Headers: ${KERNEL_HEADERS_PATH}")
message(STATUS "  Output Directory: ${CMAKE_CURRENT_BINARY_DIR}")
message(STATUS "  Build Type: ${CMAKE_BUILD_TYPE}")