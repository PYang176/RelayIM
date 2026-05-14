package snowflake

import (
	"fmt"
	"log"
	"sync"

	"github.com/bwmarrin/snowflake"
)

var (
	node *snowflake.Node
	once sync.Once
)

// Init 初始化 Snowflake 节点
func Init(workerID int64) error {
	var err error
	once.Do(func() {
		node, err = snowflake.NewNode(workerID)
		if err != nil {
			log.Fatalf(" Snowflake 初始化失败: %v", err)
		}
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
