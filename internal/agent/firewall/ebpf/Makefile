# SPDX-License-Identifier: (GPL-2.0 OR BSD-2-Clause)

# 设置编译器和基本编译选项
CC ?= gcc
CLANG ?= clang
LLC ?= llc

# eBPF 目标架构
VMLINUX := /usr/src/linux-headers-$(shell uname -r)/vmlinux

# 源文件和对象文件目录
EBPF_SRC_DIR := progs
EBPF_OBJ_DIR := objs
INCLUDE_DIR := include

# eBPF 对象文件
EBPF_OBJS := $(EBPF_OBJ_DIR)/qos_main.o

# 创建必要的目录
$(shell mkdir -p $(EBPF_OBJ_DIR))

# 编译 eBPF 对象文件的规则
$(EBPF_OBJ_DIR)/%.o: $(EBPF_SRC_DIR)/%.c
	$(CLANG) -g -O2 -target bpf -D__TARGET_ARCH_x86 -I$(INCLUDE_DIR) \
		-I/usr/include/ \
		-I/usr/src/linux-headers-$(shell uname -r)/arch/x86/include/generated/uapi \
		-I/usr/src/linux-headers-$(shell uname -r)/arch/x86/include/uapi \
		-I/usr/src/linux-headers-$(shell uname -r)/include/uapi \
		-I/usr/src/linux-headers-$(shell uname -r)/include/generated/uapi \
		-Wall -Wextra -Wunused \
		-c $< -o $@

# 编译所有 eBPF 程序
ebpf: $(EBPF_OBJS)

# 清理编译产物
clean:
	rm -rf $(EBPF_OBJ_DIR)/*

.PHONY: ebpf clean