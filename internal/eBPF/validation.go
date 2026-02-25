package eBPF

import (
	"fmt"
	"unsafe"
)

// 验证 Go 结构体与 C 结构体类型的匹配
func ValidateStructCompatibility() error {
	fmt.Println("Validating Go and C structure compatibility...")

	// 1. 验证 ACL5TupleKey 结构体
	fmt.Println("Checking ACL5TupleKey...")
	var goKey ACL5TupleKey
	cKeySize := 16 // C 中 struct acl_5tuple_key 的大小（4+4+2+2+1+1+2 = 16字节）

	if int(unsafe.Sizeof(goKey)) != cKeySize {
		return fmt.Errorf("ACL5TupleKey size mismatch: Go=%d, C=%d", unsafe.Sizeof(goKey), cKeySize)
	}
	fmt.Printf("  ✓ ACL5TupleKey size: %d bytes\n", unsafe.Sizeof(goKey))

	// 验证 ACL5TupleKey 偏移量
	assertOffset("ACL5TupleKey", "SrcIP", unsafe.Offsetof(goKey.SrcIP), 0)
	assertOffset("ACL5TupleKey", "DstIP", unsafe.Offsetof(goKey.DstIP), 4)
	assertOffset("ACL5TupleKey", "SrcPort", unsafe.Offsetof(goKey.SrcPort), 8)
	assertOffset("ACL5TupleKey", "DstPort", unsafe.Offsetof(goKey.DstPort), 10)
	assertOffset("ACL5TupleKey", "Proto", unsafe.Offsetof(goKey.Proto), 12)
	assertOffset("ACL5TupleKey", "Pad1", unsafe.Offsetof(goKey.Pad1), 13)
	assertOffset("ACL5TupleKey", "Pad2", unsafe.Offsetof(goKey.Pad2), 14)
	fmt.Println("  ✓ ACL5TupleKey offsets verified")

	// 2. 验证 ACLRuleValue 结构体
	fmt.Println("Checking ACLRuleValue...")
	var goValue ACLRuleValue
	cValueSize := 24 // C 中 struct acl_rule_value 的大小（4+4+8+8 = 24字节）

	if int(unsafe.Sizeof(goValue)) != cValueSize {
		return fmt.Errorf("ACLRuleValue size mismatch: Go=%d, C=%d", unsafe.Sizeof(goValue), cValueSize)
	}
	fmt.Printf("  ✓ ACLRuleValue size: %d bytes\n", unsafe.Sizeof(goValue))

	// 验证 ACLRuleValue 偏移量
	assertOffset("ACLRuleValue", "Action", unsafe.Offsetof(goValue.Action), 0)
	assertOffset("ACLRuleValue", "RuleID", unsafe.Offsetof(goValue.RuleID), 4)
	assertOffset("ACLRuleValue", "Bytes", unsafe.Offsetof(goValue.Bytes), 8)
	assertOffset("ACLRuleValue", "Packets", unsafe.Offsetof(goValue.Packets), 16)
	fmt.Println("  ✓ ACLRuleValue offsets verified")

	// 3. 验证 BucketState 结构体
	fmt.Println("Checking BucketState...")
	var goBucket BucketState
	cBucketSize := 56 // C 中 struct bucket_state 的大小（考虑内存对齐）

	if int(unsafe.Sizeof(goBucket)) != cBucketSize {
		return fmt.Errorf("BucketState size mismatch: Go=%d, C=%d", unsafe.Sizeof(goBucket), cBucketSize)
	}
	fmt.Printf("  ✓ BucketState size: %d bytes\n", unsafe.Sizeof(goBucket))

	// 验证 BucketState 偏移量
	assertOffset("BucketState", "RateBytesPerSec", unsafe.Offsetof(goBucket.RateBytesPerSec), 0)
	assertOffset("BucketState", "BurstBytes", unsafe.Offsetof(goBucket.BurstBytes), 8)
	assertOffset("BucketState", "Tokens", unsafe.Offsetof(goBucket.Tokens), 16)
	assertOffset("BucketState", "LastUpdateNS", unsafe.Offsetof(goBucket.LastUpdateNS), 24)
	assertOffset("BucketState", "PassBytes", unsafe.Offsetof(goBucket.PassBytes), 32)
	assertOffset("BucketState", "DropBytes", unsafe.Offsetof(goBucket.DropBytes), 40)
	// 对于占位字段的偏移检查比较特殊，我们通过RuleID字段来间接验证
	assertOffset("BucketState", "RuleID", unsafe.Offsetof(goBucket.RuleID), 52)
	fmt.Println("  ✓ BucketState offsets verified")

	// 4. 验证 FlowDetailKey 结构体
	fmt.Println("Checking FlowDetailKey...")
	var goFlowKey FlowDetailKey
	cFlowKeySize := 24 // C 中 struct flow_detail_key 的大小（4+4+4+4+2+2+1+1+2 = 24字节）

	if int(unsafe.Sizeof(goFlowKey)) != cFlowKeySize {
		return fmt.Errorf("FlowDetailKey size mismatch: Go=%d, C=%d", unsafe.Sizeof(goFlowKey), cFlowKeySize)
	}
	fmt.Printf("  ✓ FlowDetailKey size: %d bytes\n", unsafe.Sizeof(goFlowKey))

	// 验证 FlowDetailKey 偏移量
	assertOffset("FlowDetailKey", "RuleID", unsafe.Offsetof(goFlowKey.RuleID), 0)
	assertOffset("FlowDetailKey", "RuleType", unsafe.Offsetof(goFlowKey.RuleType), 4)
	assertOffset("FlowDetailKey", "SrcIP", unsafe.Offsetof(goFlowKey.SrcIP), 8)
	assertOffset("FlowDetailKey", "DstIP", unsafe.Offsetof(goFlowKey.DstIP), 12)
	assertOffset("FlowDetailKey", "SrcPort", unsafe.Offsetof(goFlowKey.SrcPort), 16)
	assertOffset("FlowDetailKey", "DstPort", unsafe.Offsetof(goFlowKey.DstPort), 18)
	assertOffset("FlowDetailKey", "Proto", unsafe.Offsetof(goFlowKey.Proto), 20)
	// 对于匿名字段的偏移检查
	fmt.Println("  ✓ FlowDetailKey offsets verified")

	// 5. 验证 FlowDetailStats 结构体
	fmt.Println("Checking FlowDetailStats...")
	var goFlowStats FlowDetailStats
	cFlowStatsSize := 24 // C 中 struct flow_detail_stats 的大小（8+8+8 = 24字节）

	if int(unsafe.Sizeof(goFlowStats)) != cFlowStatsSize {
		return fmt.Errorf("FlowDetailStats size mismatch: Go=%d, C=%d", unsafe.Sizeof(goFlowStats), cFlowStatsSize)
	}
	fmt.Printf("  ✓ FlowDetailStats size: %d bytes\n", unsafe.Sizeof(goFlowStats))

	// 验证 FlowDetailStats 偏移量
	assertOffset("FlowDetailStats", "Bytes", unsafe.Offsetof(goFlowStats.Bytes), 0)
	assertOffset("FlowDetailStats", "Packets", unsafe.Offsetof(goFlowStats.Packets), 8)
	assertOffset("FlowDetailStats", "LastSeen", unsafe.Offsetof(goFlowStats.LastSeen), 16)
	fmt.Println("  ✓ FlowDetailStats offsets verified")

	// 6. 验证 DropEventT 结构体
	fmt.Println("Checking DropEventT...")
	var goDropEvent DropEventT
	cDropEventSize := 32 // C 中 struct drop_event_t 的大小（4+4+4+4+2+2+1+1+2+8 = 32字节）

	if int(unsafe.Sizeof(goDropEvent)) != cDropEventSize {
		return fmt.Errorf("DropEventT size mismatch: Go=%d, C=%d", unsafe.Sizeof(goDropEvent), cDropEventSize)
	}
	fmt.Printf("  ✓ DropEventT size: %d bytes\n", unsafe.Sizeof(goDropEvent))

	// 验证 DropEventT 偏移量
	assertOffset("DropEventT", "RuleID", unsafe.Offsetof(goDropEvent.RuleID), 0)
	assertOffset("DropEventT", "Reason", unsafe.Offsetof(goDropEvent.Reason), 4)
	assertOffset("DropEventT", "SrcIP", unsafe.Offsetof(goDropEvent.SrcIP), 8)
	assertOffset("DropEventT", "DstIP", unsafe.Offsetof(goDropEvent.DstIP), 12)
	assertOffset("DropEventT", "SrcPort", unsafe.Offsetof(goDropEvent.SrcPort), 16)
	assertOffset("DropEventT", "DstPort", unsafe.Offsetof(goDropEvent.DstPort), 18)
	assertOffset("DropEventT", "Proto", unsafe.Offsetof(goDropEvent.Proto), 20)
	assertOffset("DropEventT", "Timestamp", unsafe.Offsetof(goDropEvent.Timestamp), 24)
	fmt.Println("  ✓ DropEventT offsets verified")

	fmt.Println("All structure compatibility checks passed!")
	return nil
}

func assertOffset(structName, fieldName string, offset, expected uintptr) {
	if offset != expected {
		panic(fmt.Sprintf("[FATAL] %s.%s offset mismatch: expected %d, got %d", structName, fieldName, expected, offset))
	}
}

// RunValidation 运行验证
func RunValidation() {
	if err := ValidateStructCompatibility(); err != nil {
		fmt.Printf("Validation failed: %v\n", err)
	} else {
		fmt.Println("✅ All validations passed!")
	}
}