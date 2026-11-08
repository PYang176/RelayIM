package snowflake

import (
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/snowflake"
)

const timestampShift = 22

var (
	node *snowflake.Node
	once sync.Once
	err  error
)

// Init 初始化 Snowflake 节点
func Init(workerID int64) error {
	once.Do(func() {
		node, err = snowflake.NewNode(workerID)
	})
	return err
}

// Generate 生成 ID
func Generate() int64 {
	if node == nil {
		panic("snowflake 未初始化")
	}
	return node.Generate().Int64()
}

// GenerateString 生成字符串形式的 ID
func GenerateString() string {
	return fmt.Sprintf("%d", Generate())
}

// Timestamp 返回雪花 ID 中包含的生成时间。
func Timestamp(id int64) time.Time {
	return time.UnixMilli((id >> timestampShift) + snowflake.Epoch)
}
