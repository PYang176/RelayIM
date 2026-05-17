package main

import (
	"fmt"
	"ginChatProject/internal/config"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/router"
	"ginChatProject/internal/task"
	"ginChatProject/utils"
	"github.com/spf13/viper"
)

func main() {
	// 初始化配置信息
	utils.InitConfig()
	// 初始化token
	utils.InitJWT()
	// 初始化redis
	utils.InitRedis()
	// 初始化 Snowflake（workerID 从配置读取）
	utils.InitSnowFlake()

	cfg := config.MySQLConfig{}
	_ = viper.UnmarshalKey("mysql", &cfg)

	// 初始化Mysql
	db, err := utils.InitMySQL(cfg)
	if err != nil {
		panic(fmt.Sprintf("数据库连接失败: %v", err))
	}

	// 启动心跳同步任务
	userRepo := repository.NewUserRepository(db)
	heartbeatTask := task.NewHeartbeatSyncTask(userRepo)
	go heartbeatTask.Start()

	r := router.Router(db)
	err = r.Run(":8081") // listens on 0.0.0.0:8080 by default
	if err != nil {
		return
	}
}

//func main() {
//	router := gin.Default()
//	router.GET("/ping", func(c *gin.Context) {
//		c.JSON(200, gin.H{
//			"message": "pong",
//		})
//	})
//	router.Run() // listens on 0.0.0.0:8080 by default
//}
