package snowflake

import (
	"testing"
	"time"

	depsnowflake "github.com/bwmarrin/snowflake"
)

func TestTimestampMatchesGeneratedID(t *testing.T) {
	node, err := depsnowflake.NewNode(1)
	if err != nil {
		t.Fatalf("创建底层 Snowflake 节点失败: %v", err)
	}
	before := time.Now().Add(-time.Second)
	id := node.Generate().Int64()
	after := time.Now().Add(time.Second)
	got := Timestamp(id)
	if got.Before(before) || got.After(after) {
		t.Fatalf("ID 时间 = %v，期望位于 %v 与 %v 之间", got, before, after)
	}
}
