package main

import (
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	defer db.Close()

	// f, _ := os.Create("gin.log")
	// gin.DefaultWriter = io.MultiWriter(f)

	router := gin.Default()

	// 打包后把前端的dist文件夹放到webserver下 并且打开下面代码即可
	// router.LoadHTMLGlob("dist/*.html")        // 添加入口index.html
	// router.LoadHTMLFiles("static/*/*")        // 添加资源路径
	// router.Static("/static", "./dist/static") // 添加资源路径
	// router.StaticFile("/", "dist/index.html") //前端接口

	//api 开头的均为前端请求
	router.GET("/api/cpuinfo", ShowCPUlogs)
	router.GET("/api/memoryinfo", ShowMemorylogs)
	router.GET("/api/netinfo", ShowNetlogs)
	router.GET("/api/robot", ShowRobotlogs)
	router.GET("/api/trans", TransInfo)
	router.GET("/api/transrecord", TransRecord)
	router.GET("/api/getprocessinfo", GetProcessInfo)
	router.GET("/api/getalalcpuinfo", GetTatalCPU)
	router.GET("/api/getallcpu", GetAllCPU)
	router.GET("/api/gettestinfo", GetTestInfo)

	router.GET("/api/delete_form", DeleteForm) //点击删除
	router.GET("/api/start", ConnectToStart)   //点击激活

	router.GET("/api/cpureport", GetCPUReport)
	router.GET("/api/memoryreport", GetMemoryReport)

	router.GET("/api/gettestplaninfo", GetTestPlanInfo)

	router.POST("/api/test_plan_set", TestPlanSet) //发送测试计划表单

	//机器人控制信息
	router.GET("/stop_test", StopTest)
	router.GET("/start_test", StartTest)
	router.GET("/register_load", RegisterLoad)

	//监控上传数据
	router.POST("/net", NetMonitor)
	router.POST("/memory", MemoryMonitor)
	router.POST("/cpu", CPUMonitor)

	//机器人上传数据
	router.POST("/statistics_info", RobotCount)
	router.POST("/trans_info", TransResult)

	//清空所有数据库
	router.GET("/api/truncate", CleanAllData)

	//检测是否超时 启动一个计时器间隔为一秒
	go ActiveCheck()

	router.Run(":8080")

}
