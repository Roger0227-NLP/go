package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-ini/ini"
)

//线程cpu占用和对应的时间点
type threadInfo struct {
	Tid  int     `json:"tid"`
	Used float64 `json:"used"`
}

//进程pid对应的tid
type pinfo struct {
	Pid     int    `json:"pid"`
	Threads []int  `json:"threads"`
	Name    string `json:"processname"`
}

type tatalCPUOfProcess struct {
	Pid  int     `json:"pid"`
	Tick int64   `json:"tick"`
	Used float64 `json:"used"`
}

type tickAndUsed struct {
	Tick int64   `json:"tick"`
	Used float64 `json:"used"`
}

type tatalCPUSend struct {
	Pid  int           `json:"pid"`
	Info []tickAndUsed `json:"info"`
}

type processInfo struct {
	Name    string     `json:"processname"`
	Pid     int        `json:"pid"`
	Tick    int64      `json:"tick"`
	Threads threadInfo `json:"threads"`
}

type memoryInfo struct {
	Pid  int    `json:"pid"`
	Tick int64  `json:"tick"`
	Name string `json:"processname"`
	Used int    `json:"used"`
}

type netInfo struct {
	Pid    int    `json:"pid"`
	Tick   int64  `json:"tick"`
	Name   string `json:"processname"`
	Dlkbps int    `json:"downlaod"`
	Upkbps int    `json:"update"`
}

type robotCount struct {
	RobotTotalNum   int `json:"robot_total_num"`
	RobotOnlineNum  int `json:"robot_online_num"`
	RecvPkgTotalNum int `json:"recv_pkg_total_num"`
	SendPkgTotalNum int `json:"send_pkg_total_num"`
}

type transInfo struct {
	TimeCost    int    `json:"time_cost"`
	TransName   string `json:"trans_name"`
	TransResult int    `json:"trans_result"`
}

type transPair struct {
	name   string
	result int
}

type transValue struct {
	num  int
	cost int
}

// result 事务结果
const (
	TRANSFAIL = iota
	TRANSSUCCESS
	TRANSERROR
	TRANSOVERTIME
)

//level 预警等级
const (
	PASS = iota
	LOW
	MEDIUM
	HIGH
	UNKNOW
)

// 测试状态
const (
	NOWORK = iota
	TESTING
	TESTOVER
)

// key pid value []tids
var _map = make(map[int][]int)

//key pid value name
var _namemap = make(map[int]string)

//key seconds value robotcountstruct
var _robotmap = make(map[int]robotCount)

//检测是否某start的机器人是否活跃 key assid val timeick
var _activePool = make(map[int]int64)

/*
//外层map的key是tick 里层map是事务名和结果组成的pair 值是它们出现的次数和花费时间总和
var _transMap = make(map[int]map[transPair]transValue)
*/
var _transMap = make(map[int][]transInfo)

//数据库连接，webserver启动时要保证数据是开启并且可以访问的状态
var db, err = sql.Open("mysql", "qingyunian:qyn@2018Yes@tcp(49.234.46.106:3306)/monitors")

var _assignInstanceID int = 0

//记录没一秒内机器人数量和发包总和 每一个记录循环清空
var _robotOnlineNum int = 0
var _robotTotalNum int = 0

//锁 下面函数会用到
var robotmutex sync.Mutex
var transmutex sync.Mutex

func findNum(num int, arr []int) bool {
	for i := 0; i < len(arr); i++ {
		if num == arr[i] {
			return true
		}
	}
	return false
}

func init() {
	if err != nil {
		fmt.Println("open err:", err)
		return
	}
	err = db.Ping()
	if err != nil {
		fmt.Println(err)
		return
	}
	/*
		rows, err := db.Query("select processname,pid,tid from cpu_infomation group by processname,pid,tid;")
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			var pid int
			var tid int
			var processname string
			err := rows.Scan(&processname, &pid, &tid)
			if err != nil {
				fmt.Printf("error: %v \n", err)
			}
			if !findNum(tid, _map[pid]) {
				_map[pid] = append(_map[pid], tid)
			}
			_namemap[pid] = processname
		}
	*/
}

//GetTestInfo 发送测试计划,状态等信息
func GetTestInfo(c *gin.Context) {

	type TestInfomation struct {
		ID        int      `json:"id"`
		Begintck  int64    `json:"begintick"`
		Endtck    int64    `json:"endtick"`
		Status    int      `json:"status"`
		RobotIP   string   `json:"robotIP"`
		TestMode  string   `json:"testmode"`
		TargetsIP []string `json:"targets"`
	}

	num := 0
	rows, err := db.Query("select count(*) from test_info")
	if err != nil {
		c.String(http.StatusForbidden, "mysql error %v", err)
		return
	}
	for rows.Next() {
		rows.Scan(&num)
	}

	infos := make([]TestInfomation, 0, num)
	rows, err = db.Query("select * from test_info")
	if err != nil {
		c.String(http.StatusForbidden, "mysql error %v", err)
		return
	}
	for rows.Next() {
		var sid int
		var testinfo TestInfomation
		rows.Scan(&testinfo.ID, &sid, &testinfo.Begintck, &testinfo.Endtck, &testinfo.Status, &testinfo.RobotIP, &testinfo.TestMode)
		//得到sid对应的目标ip个数
		ipnum := 0
		sql := fmt.Sprintf("select count(*) from target_ip where id = %d", sid)
		rows, err := db.Query(sql)
		if err != nil {
			c.String(http.StatusForbidden, "mysql error %v", err)
			return
		}
		for rows.Next() {
			rows.Scan(&ipnum)
		}

		ips := make([]string, 0, ipnum)
		//得到目标ip地址数组
		sql = fmt.Sprintf("select ip from target_ip where id = %d", sid)
		rows, err = db.Query(sql)
		if err != nil {
			c.String(http.StatusForbidden, "mysql error %v", err)
			return
		}
		for rows.Next() {
			var tmp string
			rows.Scan(&tmp)
			ips = append(ips, tmp)
		}
		testinfo.TargetsIP = ips
		//fmt.Println(testinfo)
		infos = append(infos, testinfo)
	}
	c.JSON(http.StatusOK, gin.H{
		"datas": infos,
	})
}

//TestPlanSet 将用户设置的计划信息存入数据库并安装监控到指定服务器
func TestPlanSet(c *gin.Context) {
	type TestTarget struct {
		TargetServerIP string   `json:"target_server"`
		User           string   `json:"user"`
		Port           int      `json:"port"`
		Password       string   `json:"password"`
		Processes      []string `json:"process"`
	}

	type RobotPlan struct {
		RobotServerIP string `json:"robot_server"`
		User          string `json:"user"`
		Port          int    `json:"port"`
		Password      string `json:"password"`
		//Script        string       `json:"script"`
		Targets []TestTarget `json:"targets"`
	}

	//拿到最后一行的id 然后id++
	tid := 0
	rows, err := db.Query("select id from target_ip order by id desc limit 1")
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&tid)
	}
	tid++
	fmt.Println(tid)

	//得到json数据
	var plan RobotPlan
	c.BindJSON(&plan)
	//fmt.Println(plan)
	//存储robot服务器密码
	_, err = db.Exec(
		"insert ignore into server_info (ip,port,user,pwd) values (?,?,?,?)",
		plan.RobotServerIP, plan.Port, plan.User, plan.Password)
	if err != nil {
		fmt.Printf("data insert faied, error:[%v]", err.Error())
		c.String(http.StatusForbidden, "mysql error!")
		return
	}

	// file, err := os.OpenFile("run_robot.sh", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	// if err != nil {
	// 	fmt.Println("open file failed, err:", err)
	// 	return
	// }
	// file.WriteString("nohup " + plan.Script + " &")
	// file.Close()
	// sftpClient, ok := SftpConnect(plan.User, plan.Password, plan.RobotServerIP, plan.Port)
	// if ok != nil {
	// 	c.String(http.StatusForbidden, "cant connect to robot server!")
	// 	return
	// }
	// err = UploadFile(sftpClient, "./run_robot.sh", "/root/tools")
	// if err != nil {
	// 	c.String(http.StatusForbidden, fmt.Sprintf("%v", err))
	// 	return
	// }
	// sftpClient.Close()
	//给目标服务器安装监控程序
	for i := 0; i < len(plan.Targets); i++ {
		//存储密码
		_, err = db.Exec(
			"insert ignore into server_info (ip,port,user,pwd) values (?,?,?,?)",
			plan.Targets[i].TargetServerIP, plan.Targets[i].Port, plan.Targets[i].User, plan.Targets[i].Password)
		if err != nil {
			fmt.Printf("data insert faied, error:[%v]", err.Error())
			c.String(http.StatusForbidden, "mysql error!")
			return
		}

		sftpClient, ok := SftpConnect(plan.Targets[i].User, plan.Targets[i].Password, plan.Targets[i].TargetServerIP, plan.Targets[i].Port)
		if ok != nil {
			c.String(http.StatusForbidden, "cant connect to ssh server!")
			return
		}
		defer sftpClient.Close()
		cfg, _ := ini.Load("infomation.ini")

		cfg.Section("process").Key("number").SetValue(strconv.Itoa(len(plan.Targets[i].Processes)))
		cfg.Section("local_adress").Key("ip").SetValue(plan.Targets[i].TargetServerIP)
		cfg.Section("flush_time").Key("time").SetValue(strconv.Itoa(2000))
		for j := 0; j < len(plan.Targets[i].Processes); j++ {
			cfg.Section("process").Key("name" + strconv.Itoa(j+1)).SetValue(plan.Targets[i].Processes[j])
		}
		cfg.SaveTo("infomation.ini")
		err = UploadFile(sftpClient, "./infomation.ini", "/root/tools")
		if err != nil {
			c.String(http.StatusForbidden, fmt.Sprintf("%v", err))
			return
		}
		err = UploadFile(sftpClient, "./monitor", "/root/tools")
		if err != nil {
			c.String(http.StatusForbidden, fmt.Sprintf("%v", err))
			return
		}
		err = UploadFile(sftpClient, "./run_monitor.sh", "/root/tools")
		if err != nil {
			c.String(http.StatusForbidden, fmt.Sprintf("%v", err))
			return
		}
	}

	//将targetip存入表中
	for i := 0; i < len(plan.Targets); i++ {
		_, err = db.Exec(
			"insert into `target_ip`  (`id`,`ip`) values(?,?)",
			tid, plan.Targets[i].TargetServerIP)
		if err != nil {
			fmt.Printf("data insert faied, error:[%v]", err.Error())
			c.String(http.StatusForbidden, fmt.Sprintf("%v", err))
			return
		}

	}

	sid := 0
	//先查询是否有这个robotIP对应的sid存在
	sql := fmt.Sprintf("select sid from plan_info where robot_ip = \"%s\" ", plan.RobotServerIP)
	//fmt.Println(sql)
	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&sid)
		//fmt.Println(sid)
	}
	if sid <= 0 {
		//如果不存在将robotip和目标ip的sid存入表中
		_, err = db.Exec(
			"insert into `plan_info`  (`sid`,`robot_ip`) values(?,?)",
			tid, plan.RobotServerIP)
		if err != nil {
			fmt.Printf("data insert faied, error:[%v]", err.Error())
			c.String(http.StatusForbidden, "mysql error!")
			return
		}
	} else {
		//如果存在更新这个ip对应的sid
		sql = fmt.Sprintf("update plan_info set sid = %d where robot_ip = '%s'", tid, plan.RobotServerIP)
		_, err = db.Exec(sql)
		if err != nil {
			fmt.Printf("data insert faied, error:[%v]", err.Error())
			c.String(http.StatusForbidden, "mysql error!")
			return
		}
	}

	/*
		//设置计划后生成计划表
		_, err = db.Exec(
			"insert into `test_info`  (`sid`,`begintick`,`endtick`,`status`) values(?,?,?,?)",
			tid, 0, 0, READY)
		if err != nil {
			fmt.Printf("data insert faied, error:[%v]", err.Error())
			c.String(http.StatusForbidden, "mysql error!")
			return
		}
	*/

}

//GetTestPlanInfo 得到测试计划表
func GetTestPlanInfo(c *gin.Context) {
	type RetrunData struct {
		Sid     int      `json:"sid"`
		RobotIP string   `json:"robotip"`
		Targets []string `json:"targets"`
	}

	var num int
	rows, err := db.Query("select count(*) from plan_info")
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&num)
	}

	infos := make([]RetrunData, 0, num)

	rows, err = db.Query("select * from plan_info")
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var ret RetrunData
		rows.Scan(&ret.Sid, &ret.RobotIP)
		rows, err := db.Query("select count(*) from target_ip where id = ?", ret.Sid)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&num)
		}
		ips := make([]string, 0, num)

		rows, err = db.Query("select ip from target_ip where id = ?", ret.Sid)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			var ip string
			rows.Scan(&ip)
			ips = append(ips, ip)
		}
		ret.Targets = ips
		infos = append(infos, ret)
	}
	c.JSON(http.StatusOK, gin.H{
		"infos": infos,
	})
}

//StopTest 机器人发送停止
func StopTest(c *gin.Context) {
	_robotOnlineNum = 0
	_robotTotalNum = 0
	instid, _ := strconv.Atoi(c.Query("instid"))
	robotIP := c.ClientIP()

	tick, _ := strconv.Atoi(c.Query("timestamp"))

	//找到机器人ip对应的sid
	var sid int
	sql := fmt.Sprintf("select sid from plan_info where robot_ip = \"%s\" ", robotIP)
	fmt.Println(sql)
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&sid)
	}
	fmt.Println("sid:", sid)
	//关掉sid对应的所有服务器的monitor
	var monitorip string
	sql = fmt.Sprintf("select ip from target_ip where id = %d ", sid)
	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&monitorip)
		fmt.Println(monitorip)
		var user string
		var pwd string
		var monitorport int
		sql = fmt.Sprintf("select port,user,pwd from server_info where ip = \"%s\"", monitorip)
		rows, err = db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&monitorport, &user, &pwd)
			sesstion, err := SSHConnect(user, pwd, monitorip, monitorport)
			defer sesstion.Close()
			//fmt.Println(user, pwd, monitorip, monitorport)

			if err != nil {
				c.String(http.StatusForbidden, "connect to target error %v", err)
			}
			sesstion.Run("pkill monitor") //关闭监控
		}
	}
	//更新状态表
	sql = fmt.Sprintf("update `test_info` set endtick = %d,status = %d where id = %d", tick, TESTOVER, instid)
	//fmt.Println(sql)
	_, err = db.Exec(sql)
	if err != nil {
		fmt.Printf("error:[%v]", err.Error())
		c.String(http.StatusForbidden, fmt.Sprintf("%v", err))
		return
	}

	delete(_activePool, instid)  //移除活跃的对象
	go updateTransResult(instid) //计算结果加速查找
	go updateTransRecord(instid) //计算结果加速查找表格
}

//ConnectToStart 用户点击激活
func ConnectToStart(c *gin.Context) {
	var sid int
	sid, err = strconv.Atoi(c.Query("sid"))
	if err != nil {
		c.String(http.StatusForbidden, "error args")
		return
	}

	plansid := 0
	var robotIP string
	//先查询是否有这个robotIP对应的sid存在
	sql := fmt.Sprintf("select * from plan_info where sid = %d ", sid)
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&plansid, &robotIP)
		fmt.Println(plansid, robotIP)
	}
	if plansid <= 0 {
		//不存在报错
		c.String(http.StatusForbidden, "cant find this test plan!")
		return
	}

	//运行monitor
	var monitorip string
	sql = fmt.Sprintf("select ip from target_ip where id = %d", sid)
	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&monitorip)
		var user string
		var pwd string
		var monitorport int
		sql = fmt.Sprintf("select port,user,pwd from server_info where ip = \"%s\"", monitorip)
		rows, err := db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&monitorport, &user, &pwd)
			sesstion, err := SSHConnect(user, pwd, monitorip, monitorport)
			defer sesstion.Close()
			fmt.Println(user, pwd, monitorip, monitorport)
			if err != nil {
				c.String(http.StatusForbidden, "connect to target error %v", err)
			}
			sesstion.Start("cd /root/tools && sh run_monitor.sh") //激活监控
			//fmt.Println("started")
		}
	}

	/*
		var rbtip string
		sql = fmt.Sprintf("select robot_ip from plan_info where sid = %d", sid)
		rows, err = db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&rbtip)
			var user string
			var pwd string
			var port int
			sql = fmt.Sprintf("select port,user,pwd from server_info where ip = \"%s\"", rbtip)
			rows, err = db.Query(sql)
			if err != nil {
				fmt.Printf("error: %v \n", err)
				return
			}
			for rows.Next() {
				rows.Scan(&port, &user, &pwd)
				sesstion, err := SSHConnect(user, pwd, rbtip, port)
				fmt.Println(user, pwd, rbtip, port)
				f, _ := os.OpenFile("./stderr.txt", os.O_CREATE|os.O_RDWR, 0666)
				f1, _ := os.OpenFile("./stdout.txt", os.O_CREATE|os.O_RDWR, 0666)
				defer sesstion.Close()
				sesstion.Stdout = f1
				sesstion.Stderr = f
				if err != nil {
					c.String(http.StatusForbidden, "connect to target error %v", err)
				}
				err = sesstion.Run("cd /root/tools && sh run_robot.sh")
				if err != nil {
					fmt.Println(err)
				}
			}
		}
	*/
}

//DeleteForm 删除表单
func DeleteForm(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("assid"))
	if err != nil {
		c.String(http.StatusForbidden, "error args")
		return
	}
	sql := fmt.Sprintf("delete from test_info where id = %d", id)
	_, err = db.Exec(sql)
	if err != nil {
		fmt.Printf("error:[%v]", err.Error())
		c.String(http.StatusForbidden, fmt.Sprintf("%v", err))
		return
	}
}

//StartTest 返回验证信息
func StartTest(c *gin.Context) {
	//sql := fmt.Sprintf("select id from test_info where status = %d and desc limit 1")
	robotIP := c.ClientIP()
	//if robotIP == "127.0.0.1" {
	//robotIP = "49.234.47.156"
	//}

	tick, _ := strconv.Atoi(c.Query("timestamp"))
	if tick <= 10000 {
		c.JSON(http.StatusOK, gin.H{
			"ret":        1,
			"msg":        "failed",
			"dcAddr":     "",
			"instanceid": 0,
			"testid":     strconv.Itoa(int(time.Now().Unix())),
		})
		return
	}

	mode := c.Query("test_mode")
	if mode == "" {
		c.JSON(http.StatusOK, gin.H{
			"ret":        1,
			"msg":        "failed",
			"dcAddr":     "",
			"instanceid": 0,
			"testid":     strconv.Itoa(int(time.Now().Unix())),
		})
		return
	}

	fmt.Println(robotIP)
	//拿到机器人ip对应的sid
	var sid int
	sql := fmt.Sprintf("select sid from plan_info where robot_ip = '%s'", robotIP)
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}
	for rows.Next() {
		rows.Scan(&sid)
	}
	if sid <= 0 {
		c.String(http.StatusForbidden, "search robot ip error")
		return
	}

	//生成测试表格
	_, err = db.Exec(
		"insert into `test_info`  (`sid`,`begintick`,`endtick`,`status`,`robotip`,`testmode`) values(?,?,?,?,?,?) ",
		sid, tick, 0, TESTING, robotIP, mode)
	if err != nil {
		fmt.Printf("data insert faied, error:[%v]", err.Error())
		c.String(http.StatusForbidden, "mysql error!")
		return
	}

	//得到刚才生成的instanceid
	var assignInstanceID int
	rows, err = db.Query("select id from test_info order by id desc limit 1 ")
	if err != nil {
		fmt.Printf("error:[%v]", err.Error())
		c.String(http.StatusForbidden, fmt.Sprintf("%v", err))
		return
	}
	for rows.Next() {
		rows.Scan(&assignInstanceID)
	}
	_activePool[assignInstanceID] = time.Now().Unix()
	/*
		//跟新状态表
		sql = fmt.Sprintf("update `test_info` set begintick = %d,status = %d where sid = %d and id = %d", tick, TESTING, sid, assignInstanceID)
		_, err = db.Exec(sql)
		if err != nil {
			fmt.Printf("error:[%v]", err.Error())
			c.String(http.StatusForbidden, fmt.Sprintf("%v", err))
			return
		}
	*/

	c.JSON(http.StatusOK, gin.H{
		"ret":        0,
		"msg":        "success",
		"dcAddr":     "",
		"instanceid": assignInstanceID,
		"testid":     strconv.Itoa(int(time.Now().Unix())),
	})
}

//GetCPUReport 发送测试报告cpureport?begintick=&endtick=&targetip=&id=
func GetCPUReport(c *gin.Context) {
	type CPUUseLevel struct {
		Name  string `json:"name"`
		Pid   int    `json:"pid"`
		Level int    `json:"level"`
		Tick  int64  `json:"tick"`
	}

	targetIP := c.Query("targetip")
	if targetIP == "" {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	begintick, _ := strconv.Atoi(c.Query("begintick"))
	if begintick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	endtick, _ := strconv.Atoi(c.Query("endtick"))
	if endtick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	//读取配置文件
	cfg, err := ini.Load("report.ini")

	if err != nil {
		c.String(http.StatusOK, "configure file report.ini error please check it")
	}
	lowline, err := cfg.Section("cpu").Key("lowline").Float64()
	if err != nil {
		c.String(http.StatusOK, "configure file report.ini error please check it")
	}
	mediumline, err := cfg.Section("cpu").Key("mediumline").Float64()
	if err != nil {
		c.String(http.StatusOK, "configure file report.ini error please check it")
	}
	highline, err := cfg.Section("cpu").Key("highline").Float64()
	if err != nil {
		c.String(http.StatusOK, "configure file report.ini error please check it")
	}

	//查询进程信息
	_, namemap := getprocessinfo(targetIP, begintick, endtick)

	aCPUuselevel := make([]CPUUseLevel, 0, len(namemap))
	for key, name := range namemap {
		var vCPUuselevel CPUUseLevel
		vCPUuselevel.Name = name
		vCPUuselevel.Pid = key
		num, useds, ticks := getmaxthreadcpu(targetIP, key, begintick, endtick)
		max := useds[0]
		vCPUuselevel.Tick = 0
		for i := 0; i < num; i++ {
			if max < useds[i] {
				max = useds[i]
				vCPUuselevel.Tick = ticks[i]
			}
		}
		if max >= lowline && max < mediumline {
			vCPUuselevel.Level = LOW
		} else if max >= mediumline && max < highline {
			vCPUuselevel.Level = MEDIUM
		} else if max >= highline {
			vCPUuselevel.Level = HIGH
		}
		aCPUuselevel = append(aCPUuselevel, vCPUuselevel)
	}
	c.JSON(http.StatusOK, gin.H{
		"datas": aCPUuselevel,
	})
}

//GetMemoryReport 内存报告memoryreport?begintick=&endtick=&targetip=
func GetMemoryReport(c *gin.Context) {

	type MemoryUsed struct {
		Name  string  `json:"name"`
		Pid   int     `json:"pid"`
		Level int     `json:"level"`
		LRb   float32 `json:"lrb"`
	}

	targetIP := c.Query("targetip")
	if targetIP == "" {
		c.JSON(http.StatusNotFound, "ip error ")
		return
	}
	fmt.Println(targetIP)
	begintick, _ := strconv.Atoi(c.Query("begintick"))
	if begintick <= 0 {
		c.JSON(http.StatusNotFound, "begintick")
		return
	}

	endtick, _ := strconv.Atoi(c.Query("endtick"))
	if endtick <= 0 {
		c.JSON(http.StatusNotFound, "endtick")
		return
	}

	//读取配置文件
	cfg, err := ini.Load("report.ini")
	if err != nil {
		c.String(http.StatusForbidden, "configure file report.ini error please check it")
	}
	deadline, err := cfg.Section("memory").Key("deadline").Float64()
	if err != nil {
		c.String(http.StatusForbidden, "configure file report.ini error please check it")
	}
	minutes, err := cfg.Section("memory").Key("minutes").Int()
	if err != nil {
		c.String(http.StatusForbidden, "configure file report.ini error please check it")
	}
	//fmt.Println(deadline, minutes)
	_, namemap := getprocessinfo(targetIP, begintick, endtick)
	//fmt.Println(namemap)
	//查询内存消耗
	aMemoryPort := make([]MemoryUsed, 0, len(namemap))
	for key, name := range namemap {
		var memoryport MemoryUsed
		memoryport.Name = name
		memoryport.Pid = key
		if begintick+60*minutes < endtick {
			ticks, useds := getMemoryInfos(targetIP, key, endtick-60*minutes, endtick)
			_, b := LinearRegressionInt(ticks, useds)
			memoryport.LRb = float32(b)
			if b > deadline {
				memoryport.Level = HIGH
			} else {
				memoryport.Level = PASS
			}
		} else {
			memoryport.Level = UNKNOW
		}
		aMemoryPort = append(aMemoryPort, memoryport)
	}
	c.JSON(http.StatusOK, gin.H{
		"datas": aMemoryPort,
	})
}

//得到进程信息
func getprocessinfo(targetIP string, begintick int, endtick int) (pidmap map[int][]int, namemap map[int]string) {
	// key pid value []tids
	pidmap = make(map[int][]int)

	//key pid value name
	namemap = make(map[int]string)

	type ReturnType struct {
		ProcessName string `json:"processname"`
		Pid         int    `json:"pid"`
		Tids        []int  `json:"tids"`
	}
	//fmt.Println(targetIP, begintick, endtick)
	ip := InetAtoN(targetIP)
	sql := fmt.Sprintf("select processname,pid,tid from cpu_infomation where timetick > %d and timetick <= %d group by pid,tid,processname,ip having ip = %d", begintick, endtick, ip)
	//fmt.Println(sql)
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var pid int
		var tid int
		var processname string
		err := rows.Scan(&processname, &pid, &tid)
		if err != nil {
			fmt.Printf("error: %v \n", err)
		}
		if len(pidmap[pid]) == 0 {
			pidmap[pid] = make([]int, 0, 100)
		}
		pidmap[pid] = append(pidmap[pid], tid)

		namemap[pid] = processname
	}

	return pidmap, namemap
}

//RegisterLoad 注册
func RegisterLoad(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ret":     0,
		"message": "success",
	})
}

//GetTatalCPU 发送进程对应最高cpu占用线程
func GetTatalCPU(c *gin.Context) {

	targetIP := c.Query("targetip")
	if targetIP == "" {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	pid, _ := strconv.Atoi(c.Query("pid"))
	if pid <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	begintick, _ := strconv.Atoi(c.Query("begintick"))
	if begintick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	endtick, _ := strconv.Atoi(c.Query("endtick"))
	if endtick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	num, usedarray, tickarray := getmaxthreadcpu(targetIP, pid, begintick, endtick)
	c.JSON(http.StatusOK, gin.H{
		"length": num,
		"ticks":  tickarray,
		"useds":  usedarray,
	})
}

func getmaxthreadcpu(targetIP string, pid int, begintick int, endtick int) (int, []float64, []int64) {

	ip := InetAtoN(targetIP)
	//查询总行数
	sql := fmt.Sprintf("select count(*) from (select  timetick as cpu from cpu_infomation where ip = %d and pid = %d and timetick > %d and timetick <= %d group by timetick,pid order by timetick) as A", ip, pid, begintick, endtick)
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}
	var num int
	for rows.Next() {
		rows.Scan(&num)
	}
	//得到每一行
	sql = fmt.Sprintf("select timetick,MAX(cpu_use) as cpu from cpu_infomation where ip = %d and pid = %d and timetick > %d and timetick <= %d group by timetick,pid order by timetick", ip, pid, begintick, endtick)

	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}
	usedarray := make([]float64, 0, num)
	tickarray := make([]int64, 0, num)
	for rows.Next() {
		var used float64
		var tick int64
		rows.Scan(&tick, &used)
		usedarray = append(usedarray, used)
		tickarray = append(tickarray, tick)
	}
	return num, usedarray, tickarray
}

//TransInfo 返回事务数据 sid=
func TransInfo(c *gin.Context) {

	assid, _ := strconv.Atoi(c.Query("sid"))

	type ReturnDatas struct {
		Name        string  `json:"name"`
		Sum         int     `json:"sum"`
		Success     int     `json:"success"`
		Failed      int     `json:"failed"`
		Error       int     `json:"error"`
		TimeOut     int     `json:"timeout"`
		SuccessRate float32 `json:"successrate"`
		Avg         float32 `json:"tps"`
		MaxCost     int     `json:"maxcost"`
		MinCost     int     `json:"mincost"`
		Percent50   int     `json:"50percent"`
		Percent75   int     `json:"75percent"`
		Percent90   int     `json:"90percent"`
	}

	var count int
	rows, err := db.Query("select count(*) from trans_result where instanceid = ?", assid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&count)
	}

	if count > 0 {
		returnDatas := make([]ReturnDatas, 0, count)
		rows, err := db.Query("select * from trans_result where instanceid = ?", assid)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			var tmpID int
			var ret ReturnDatas
			rows.Scan(&tmpID, &ret.Sum, &ret.Success, &ret.Failed,
				&ret.TimeOut, &ret.Error, &ret.SuccessRate, &ret.Avg, &ret.MaxCost,
				&ret.MinCost, &ret.Percent50, &ret.Percent75, &ret.Percent90, &ret.Name)
			returnDatas = append(returnDatas, ret)
		}
		//fmt.Println(returnDatas)
		c.JSON(http.StatusOK, gin.H{
			"infos": returnDatas,
		})
		return
	}

	rows, err = db.Query("select count(*) from (select name,result as num from trans_info group by result,name,assigninstanceid having assigninstanceid = ?) as A", assid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	var size int
	for rows.Next() {
		rows.Scan(&size)
	}
	returnDatas := make([]ReturnDatas, 0, size)
	res := make(map[string]*ReturnDatas)
	percents := make(map[string]int)

	times := 0
	rows, err = db.Query("select count(*) from (select timetick from trans_info group by timetick,assigninstanceid having assigninstanceid = ?) as A;", assid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&times)
	}

	//2s
	rows, err = db.Query("select name,result,count(result) as num from trans_info group by result,name,assigninstanceid having assigninstanceid = ?", assid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var name string
		var result int
		var num int
		rows.Scan(&name, &result, &num)
		if res[name] == nil {
			res[name] = new(ReturnDatas)
			percents[name] = 0
		}
		if result == TRANSFAIL {
			res[name].Failed = num
		}
		if result == TRANSSUCCESS {
			res[name].Success = num
		}
		if result == TRANSOVERTIME {
			res[name].TimeOut = num
		}
		if result == TRANSERROR {
			res[name].Error = num
		}
	}

	rows, err = db.Query("select name,max(A.m) as max from (select name,max(cost) as m from trans_info group by name,assigninstanceid having assigninstanceid = ?) as A group by A.name", assid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var name string
		var max int
		rows.Scan(&name, &max)
		res[name].MaxCost = max
	}

	rows, err = db.Query("select name,min(A.m) as min from (select name,min(cost) as m from trans_info group by name,assigninstanceid having assigninstanceid = ?) as A group by A.name", assid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var name string
		var min int
		rows.Scan(&name, &min)
		res[name].MinCost = min
	}

	//2s
	rows, err = db.Query("select * from (select name,cost,count(cost) as num from trans_info group by name,cost,assigninstanceid having assigninstanceid = ?) as A  order by A.cost", assid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var name string
		var cost int
		var num int
		rows.Scan(&name, &cost, &num)
		res[name].Sum = res[name].Success + res[name].Error + res[name].Failed + res[name].TimeOut
		res[name].Avg = float32(res[name].Sum) / float32(times)
		percents[name] += num
		if float32(percents[name]) >= float32(res[name].Sum)*0.5 && float32(percents[name]) < float32(res[name].Sum)*0.75 {
			(*res[name]).Percent50 = cost
		}
		if float32(percents[name]) >= float32(res[name].Sum)*0.75 && float32(percents[name]) < float32(res[name].Sum)*0.9 {
			(*res[name]).Percent75 = cost
		}
		if float32(percents[name]) >= float32(res[name].Sum)*0.9 {
			(*res[name]).Percent90 = cost
		}
	}

	for key, val := range res {
		var tmp ReturnDatas
		if val.Success == val.Success+val.Error+val.Failed+val.TimeOut && val.Success == 0 {
			val.SuccessRate = 100
		} else {
			val.SuccessRate = 100 * float32(val.Success) / float32(val.Success+val.Error+val.Failed+val.TimeOut)
		}
		tmp = *val
		tmp.Name = key
		returnDatas = append(returnDatas, tmp)
	}

	c.JSON(http.StatusOK, gin.H{
		"infos": returnDatas,
	})
}

//TransRecord 发送给前端事务图表信息
func TransRecord(c *gin.Context) {
	sid, _ := strconv.Atoi(c.Query("sid"))
	//fmt.Println(sid)
	name := c.Query("name")
	begintick, _ := strconv.Atoi(c.Query("begintck"))
	endtick, _ := strconv.Atoi(c.Query("endtick"))
	isGetTime, _ := strconv.ParseBool(c.DefaultQuery("getinfo", "false"))
	if isGetTime {
		var starttime int64
		var endtime int64
		sql := fmt.Sprintf("select timetick from trans_info where assigninstanceid = %d order by timetick limit 1", sid)
		//fmt.Println(sid)
		rows, err := db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&starttime)
		}
		sql = fmt.Sprintf("select timetick from trans_info where assigninstanceid = %d order by timetick desc limit 1", sid)
		rows, err = db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&endtime)
		}
		var size int
		sql = fmt.Sprintf("select count(*) from (select name from trans_info group by name,assigninstanceid having assigninstanceid = %d) as A", sid)
		rows, err = db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&size)
		}
		names := make([]string, 0, size)
		sql = fmt.Sprintf("select name from trans_info where assigninstanceid = %d  group by name", sid)
		rows, err = db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			var name string
			rows.Scan(&name)
			names = append(names, name)
		}

		c.JSON(http.StatusOK, gin.H{
			"starttime": starttime,
			"endtime":   endtime,
			"names":     names,
		})
	} else {

		sql := fmt.Sprintf("select count(*) from trans_record_result where instanceid = %d and name = \"%s\"", sid, name)
		var count int
		rows, err := db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&count)
		}

		if count > 0 {
			ticks := make([]int, 0, count)
			nums := make([]int, 0, count)
			sql := fmt.Sprintf("select tick,nums from trans_record_result where tick > %d and tick <= %d and instanceid = %d and name = \"%s\" order by tick", begintick, endtick, sid, name)
			rows, err := db.Query(sql)
			if err != nil {
				fmt.Printf("error: %v \n", err)
				return
			}
			for rows.Next() {
				var tick int
				var num int
				rows.Scan(&tick, &num)
				ticks = append(ticks, tick)
				nums = append(nums, num)
			}
			c.JSON(http.StatusOK, gin.H{
				"ticks": ticks,
				"nums":  nums,
			})
			return
		}

		var size int

		sql = fmt.Sprintf("select count(*) from (select timetick as num from trans_info  where assigninstanceid = %d and name = '%s' and timetick > %d and timetick <= %d group by timetick,name) as A", sid, name, begintick, endtick)
		rows, err = db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&size)
		}

		ticks := make([]int64, 0, size)
		nums := make([]int, 0, size)

		sql = fmt.Sprintf("select timetick,count(*) as num from trans_info group by timetick,name,assigninstanceid having name = '%s' and timetick > %d and timetick <= %d and assigninstanceid = %d", name, begintick, endtick, sid)
		rows, err = db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			var tick int64
			var num int
			rows.Scan(&tick, &num)
			ticks = append(ticks, tick)
			nums = append(nums, num)
		}

		c.JSON(http.StatusOK, gin.H{
			"ticks": ticks,
			"nums":  nums,
		})
	}
}

//GetProcessInfo 发送给前端进程信息 targetip=&begintick=&endtick=
func GetProcessInfo(c *gin.Context) {
	type ReturnType struct {
		ProcessName string `json:"processname"`
		Pid         int    `json:"pid"`
		Tids        []int  `json:"tids"`
	}
	targetIP := c.Query("targetip")
	if targetIP == "" {
		c.String(http.StatusForbidden, "error args")
	}

	begintick, _ := strconv.Atoi(c.Query("begintick"))
	if begintick < 100000 {
		c.String(http.StatusForbidden, "error args")
	}
	endtick, _ := strconv.Atoi(c.Query("endtick"))
	if endtick < 100000 {
		c.String(http.StatusForbidden, "error args")
	}

	pidmap, namemap := getprocessinfo(targetIP, begintick, endtick)
	rets := make([]ReturnType, 0, 100)

	for key, val := range namemap {
		var ret ReturnType
		ret.Pid = key
		ret.ProcessName = val
		ret.Tids = pidmap[key]
		rets = append(rets, ret)
	}
	c.JSON(http.StatusOK, gin.H{
		"datas": rets,
	})
}

func removeRepByMap(slc []int64) []int64 {
	result := []int64{}         //存放返回的不重复切片
	tempMap := map[int64]byte{} // 存放不重复主键
	for _, e := range slc {
		l := len(tempMap)
		tempMap[e] = 0 //当e存在于tempMap中时，再次添加是添加不进去的，，因为key不允许重复
		//如果上一行添加成功，那么长度发生变化且此时元素一定不重复
		if len(tempMap) != l { // 加入map后，map长度变化，则元素不重复
			result = append(result, e) //当元素不重复时，将元素添加到切片result中
		}
	}
	return result
}

//ShowCPUlogs 发送CPU日志 /api/cpuinfo?begintick=&endtick=&pid=&tid=&targetip=
func ShowCPUlogs(c *gin.Context) {

	var num int

	targetIP := c.Query("targetip")
	if targetIP == "" {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	pid, _ := strconv.Atoi(c.Query("pid"))
	if pid <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	begintick, _ := strconv.Atoi(c.Query("begintick"))
	if begintick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	tid, _ := strconv.Atoi(c.Query("tid"))
	if begintick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	endtick, _ := strconv.Atoi(c.Query("endtick"))
	if endtick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	ip := InetAtoN(targetIP)
	sql := fmt.Sprintf("select count(*) from (select timetick,tid,cpu_use from cpu_infomation where ip = %d and pid = %d and timetick > %d and timetick <= %d and tid = %d order by timetick) as A", ip, pid, begintick, endtick, tid)
	//fmt.Println(sql)
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}

	for rows.Next() {
		rows.Scan(&num)
	}

	sql = fmt.Sprintf("select timetick,cpu_use from cpu_infomation where ip = %d and pid = %d and timetick > %d and timetick <= %d  and tid = %d order by timetick ", ip, pid, begintick, endtick, tid)
	//fmt.Println(sql)
	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}

	useds := make([]float64, 0, num)
	ticks := make([]int64, 0, num)

	for rows.Next() {
		var tick int64
		var used float64
		rows.Scan(&tick, &used)
		useds = append(useds, used)
		ticks = append(ticks, tick)
	}

	c.JSON(http.StatusOK, gin.H{
		"ticks": ticks,
		"useds": useds,
	})
}

//ShowMemorylogs 发送内存日志 targetip=&pid=&begintick=&endtick=
func ShowMemorylogs(c *gin.Context) {

	targetIP := c.Query("targetip")
	if targetIP == "" {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	pid, _ := strconv.Atoi(c.Query("pid"))
	if pid <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	begintick, _ := strconv.Atoi(c.Query("begintick"))
	if begintick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	endtick, _ := strconv.Atoi(c.Query("endtick"))
	if endtick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	ticks, useds := getMemoryInfos(targetIP, pid, begintick, endtick)
	c.JSON(http.StatusOK, gin.H{
		"ticks": ticks,
		"useds": useds,
	})
}

func getMemoryInfos(targetIP string, pid int, begintick int, endtick int) ([]int64, []float64) {
	sql := fmt.Sprintf("select count(*) from (select timetick,pid,memory_use from memory_infomation where ip = \"%s\" and pid = %d and timetick > %d and timetick <= %d order by timetick ) as A", targetIP, pid, begintick, endtick)
	var num int
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}

	for rows.Next() {
		rows.Scan(&num)
	}

	ticks := make([]int64, 0, num)
	useds := make([]float64, 0, num)

	sql = fmt.Sprintf("select timetick,pid,memory_use from memory_infomation where ip = \"%s\" and pid = %d and timetick > %d and timetick <= %d order by timetick ", targetIP, pid, begintick, endtick)

	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error265: %v \n", err)
	}
	for rows.Next() {
		var pid int
		var tick int64
		var used float64
		rows.Scan(&tick, &pid, &used)
		useds = append(useds, used)
		ticks = append(ticks, tick)
	}
	return ticks, useds
}

//ShowNetlogs 发送网络流量日志
func ShowNetlogs(c *gin.Context) {
	targetIP := c.Query("targetip")
	if targetIP == "" {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	pid, _ := strconv.Atoi(c.Query("pid"))
	if pid <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	begintick, _ := strconv.Atoi(c.Query("begintick"))
	if begintick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	endtick, _ := strconv.Atoi(c.Query("endtick"))
	if endtick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	sql := fmt.Sprintf("select count(*) from (select timetick,pid,download_speed,update_speed from net_flow where ip = \"%s\" and pid = %d and timetick > %d and timetick <= %d order by timetick ) as A", targetIP, pid, begintick, endtick)

	var num int
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}

	for rows.Next() {
		rows.Scan(&num)
	}

	ticks := make([]int64, 0, num)
	downloadSpeeds := make([]int, 0, num)
	updateSpeeds := make([]int, 0, num)
	sql = fmt.Sprintf("select timetick,pid,download_speed,update_speed from net_flow where ip = \"%s\" and pid = %d and timetick > %d and timetick <= %d order by timetick ", targetIP, pid, begintick, endtick)

	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}
	for rows.Next() {
		var pid int
		var tick int64
		var downloadSpeed int
		var updateSpeed int
		rows.Scan(&tick, &pid, &downloadSpeed, &updateSpeed)
		downloadSpeeds = append(downloadSpeeds, downloadSpeed)

		updateSpeeds = append(updateSpeeds, updateSpeed)
		ticks = append(ticks, tick)
	}
	//fmt.Println(updateSpeeds)
	c.JSON(http.StatusOK, gin.H{
		"ticks":          ticks,
		"downloadSpeeds": downloadSpeeds,
		"updateSpeeds":   updateSpeeds,
	})
}

//deleteExtraSpace
func deleteExtraSpace(s string) string {
	//删除字符串中的多余空格，有多个空格时，仅保留一个空格
	s1 := strings.Replace(s, "	", " ", -1)      //替换tab为空格
	regstr := "\\s{2,}"                         //两个及两个以上空格的正则表达式
	reg, _ := regexp.Compile(regstr)            //编译正则表达式
	s2 := make([]byte, len(s1))                 //定义字符数组切片
	copy(s2, s1)                                //将字符串复制到切片
	spcIndex := reg.FindStringIndex(string(s2)) //在字符串中搜索
	for len(spcIndex) > 0 {                     //找到适配项
		s2 = append(s2[:spcIndex[0]+1], s2[spcIndex[1]:]...) //删除多余空格
		spcIndex = reg.FindStringIndex(string(s2))           //继续在字符串中搜索
	}
	return string(s2)
}

func getFile(path, filename string) (string, error) {
	data, err := ioutil.ReadFile(path + filename)
	if err != nil {
		fmt.Println("File reading error: ", err)
		return "", err
	}
	return string(data), nil
}

//GetAllCPU 发送cpu占用总和
func GetAllCPU(c *gin.Context) {

	targetIP := c.Query("targetip")
	if targetIP == "" {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	pid, _ := strconv.Atoi(c.Query("pid"))
	if pid <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	begintick, _ := strconv.Atoi(c.Query("begintick"))
	if begintick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}

	endtick, _ := strconv.Atoi(c.Query("endtick"))
	if endtick <= 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	ip := InetAtoN(targetIP)
	sql := fmt.Sprintf("select count(*) from (select timetick,sum(cpu_use) as used from cpu_infomation where ip = %d and pid = %d and timetick > %d and timetick <= %d group by timetick,pid) as A", ip, pid, begintick, endtick)

	var num int
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}

	for rows.Next() {
		rows.Scan(&num)
	}

	sql = fmt.Sprintf("select timetick,sum(cpu_use) as used from cpu_infomation where ip = %d and pid = %d and timetick > %d and timetick <= %d group by timetick,pid", ip, pid, begintick, endtick)

	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
	}

	useds := make([]float64, 0, num)
	ticks := make([]int64, 0, num)
	for rows.Next() {
		var tick int64
		var used float64
		rows.Scan(&tick, &used)
		useds = append(useds, used)
		ticks = append(ticks, tick)
	}

	c.JSON(http.StatusOK, gin.H{
		"ticks": ticks,
		"useds": useds,
	})
}

//ShowRobotlogs 发送机器人信息 sid=
func ShowRobotlogs(c *gin.Context) {

	sid, _ := strconv.Atoi(c.Query("sid"))
	if sid <= 0 {
		c.String(http.StatusForbidden, "args error : sid")
		return
	}
	isGetTime, _ := strconv.ParseBool(c.DefaultQuery("gettime", "false"))
	if isGetTime {
		var starttime int64
		var endtime int64
		sql := fmt.Sprintf("select timetick from robot_info where assigninstanceid = %d order by timetick limit 1", sid)
		rows, err := db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&starttime)
		}
		sql = fmt.Sprintf("select timetick from robot_info where assigninstanceid = %d order by timetick desc limit 1", sid)
		rows, err = db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&endtime)
		}
		c.JSON(http.StatusOK, gin.H{
			"starttime": starttime,
			"endtime":   endtime,
		})
	} else {
		begintick, _ := strconv.Atoi(c.Query("begintick"))
		if begintick <= 0 {
			c.JSON(http.StatusNotFound, nil)
			return
		}

		endtick, _ := strconv.Atoi(c.Query("endtick"))
		if endtick <= 0 {
			c.JSON(http.StatusNotFound, nil)
			return
		}

		sql := fmt.Sprintf("select count(*) from (select * from robot_info where  assigninstanceid = %d and timetick > %d and timetick <= %d) as A", sid, begintick, endtick)
		var num int
		rows, err := db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
		}

		for rows.Next() {
			rows.Scan(&num)
		}

		sql = fmt.Sprintf("select timetick,totalnum,onlinenum,recvpkg,sendpkg from robot_info where  assigninstanceid = %d and timetick > %d and timetick <= %d order by timetick", sid, begintick, endtick)
		rows, err = db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
		}

		totalnums := make([]int, 0, num)
		onlinenums := make([]int, 0, num)
		recvpkgs := make([]int, 0, num)
		sendpkgs := make([]int, 0, num)
		ticks := make([]int64, 0, num)
		for rows.Next() {
			var tick int64
			var totalnum int
			var onlinenum int
			var recvpkg int
			var sendpkg int
			rows.Scan(&tick, &totalnum, &onlinenum, &recvpkg, &sendpkg)
			totalnums = append(totalnums, totalnum)
			onlinenums = append(onlinenums, onlinenum)
			recvpkgs = append(recvpkgs, recvpkg)
			sendpkgs = append(sendpkgs, sendpkg)
			ticks = append(ticks, tick)
		}
		c.JSON(http.StatusOK, gin.H{
			"totalnums":  totalnums,
			"onlinenums": onlinenums,
			"recvpkgs":   recvpkgs,
			"sendpkgs":   sendpkgs,
			"ticks":      ticks,
		})
	}
}

//CPUMonitor 将cpu采集的信息存入数据库
func CPUMonitor(c *gin.Context) {

	pid := c.Query("pid")
	processname := c.Query("processname")
	values := c.PostForm("values")
	machineip := c.Query("ip")         //测试机器IP
	values = deleteExtraSpace(values)  //删除所有连续的空格
	logs := strings.Split(values, "*") //拿到每一条数据
	ip := InetAtoN(machineip)
	for i := 0; i < len(logs); i++ {

		var formatTimeStr string
		info := strings.Split(logs[i], " ")
		if len(info) >= 2 {
			formatTimeStr = info[0] + " " + info[1]
		} else {
			continue
		}

		var tick int64
		var tid int
		var used float64
		formatTime, err := time.ParseInLocation("2006-01-02 15:04:05", formatTimeStr, time.Local)

		if err == nil {
			tick = formatTime.Unix()
		} else {
			fmt.Fprintf(gin.DefaultErrorWriter, "%v\n", err)
		}

		tids := make([]int, 0, 100)
		for j := 2; j < len(info)-1; j += 2 {

			tid, _ = strconv.Atoi(info[j])
			tids = append(tids, tid)
			used, _ = strconv.ParseFloat(info[j+1], 32)

			_, err = db.Exec(
				"insert into `cpu_infomation` (`timetick`,`processname`,`pid`,`tid`,`cpu_use`,`ip`) values(?,?,?,?,?,?)",
				tick, processname, pid, tid, used, ip)
			if err != nil {
				fmt.Printf("data insert faied, error:[%v]", err.Error())
				return
			}

			id, _ := strconv.Atoi(pid)
			_map[id] = tids            //pid 和tids 的索引表
			_namemap[id] = processname //pid和name的索引表
		}
	}
}

//MemoryMonitor 内存采集的信息
func MemoryMonitor(c *gin.Context) {
	pid := c.Query("pid")                 //进程pid
	processname := c.Query("processname") //拿到进程名
	machineip := c.Query("ip")            //测试机器IP
	values := c.PostForm("values")
	values = deleteExtraSpace(values)  //删除所有连续的空格
	logs := strings.Split(values, "*") //拿到每一条数据
	for i := 0; i < len(logs); i++ {
		var formatTimeStr string
		info := strings.Split(logs[i], " ")
		if len(info) >= 2 {
			formatTimeStr = info[0] + " " + info[1]
		} else {
			continue
		}
		var tick int64
		var kb int
		formatTime, err := time.ParseInLocation("2006-01-02 15:04:05", formatTimeStr, time.Local)
		if err == nil {
			tick = formatTime.Unix()
		} else {
			fmt.Fprintf(gin.DefaultErrorWriter, "%v\n", err)
		}

		kb, _ = strconv.Atoi(info[2])
		//存入数据库
		_, err = db.Exec(
			"insert into `memory_infomation` (`timetick`,`processname`,`pid`,`memory_use`,`ip`) values(?,?,?,?,?)",
			tick, processname, pid, kb, machineip)

		if err != nil {
			fmt.Printf("data insert faied, error:[%v]", err.Error())
			return
		}
	}
}

//NetMonitor 采集的网速信息
func NetMonitor(c *gin.Context) {
	pid := c.Query("pid")                 //进程pid
	processname := c.Query("processname") //拿到进程名
	machineip := c.Query("ip")            //测试机器IP
	values := c.PostForm("values")
	values = deleteExtraSpace(values)  //删除所有连续的空格
	logs := strings.Split(values, "*") //拿到每一条数据
	for i := 0; i < len(logs); i++ {
		var formatTimeStr string
		info := strings.Split(logs[i], " ")
		if len(info) >= 2 {
			formatTimeStr = info[0] + " " + info[1]
		} else {
			continue
		}
		var tick int64
		var dlkb float64
		var upkb float64
		formatTime, err := time.ParseInLocation("2006-01-02 15:04:05", formatTimeStr, time.Local)
		if err == nil {
			tick = formatTime.Unix() //字符串转时间戳
		} else {
			fmt.Fprintf(gin.DefaultErrorWriter, "%v\n", err)
		}

		upkb, _ = strconv.ParseFloat(info[2], 64)
		dlkb, _ = strconv.ParseFloat(info[3], 64)
		//fmt.Printf("pid = %s processname = %s dl %f kbps up %f kbps\n", pid, processname, dlkb, upkb)
		//存入数据库
		_, err = db.Exec(
			"insert into `net_flow`  (`timetick`,`processname`,`pid`,`download_speed`,`update_speed`,`ip`) values(?,?,?,?,?,?)",
			tick, processname, pid, dlkb, upkb, machineip)

		if err != nil {
			fmt.Printf("data insert faied, error:[%v]", err.Error())
			return
		}
	}
}

//RobotCount 机器人人数和收发包率
func RobotCount(c *gin.Context) {

	type data struct {
		Data robotCount `json:"data"`
	}

	tick, _ := strconv.Atoi(c.Query("timestamp"))
	if tick <= 1000 {
		c.String(http.StatusNotFound, "error args")
	}
	instanceid, _ := strconv.Atoi(c.Query("instanceid"))
	var recv data
	_ = c.BindJSON(&recv)

	robotmutex.Lock()
	defer robotmutex.Unlock()
	var count robotCount
	_robotOnlineNum += recv.Data.RobotOnlineNum
	_robotTotalNum += recv.Data.RobotTotalNum
	count.RecvPkgTotalNum = _robotmap[tick].RecvPkgTotalNum + recv.Data.RecvPkgTotalNum
	count.RobotOnlineNum = _robotOnlineNum
	count.RobotTotalNum = _robotTotalNum
	count.SendPkgTotalNum = _robotmap[tick].SendPkgTotalNum + recv.Data.SendPkgTotalNum

	_robotmap[tick] = count

	if len(_robotmap) > 8 {
		for key, val := range _robotmap {
			if key%2 == 0 {
				_, err = db.Exec(
					"insert into `robot_info`  (`timetick`,`totalnum`,`onlinenum`,`recvpkg`,`sendpkg`,`assigninstanceid`) values(?,?,?,?,?,?)",
					key, val.RobotTotalNum, val.RobotOnlineNum, val.RecvPkgTotalNum, val.SendPkgTotalNum, instanceid)
				if err != nil {
					fmt.Printf("data insert faied, error:[%v]", err.Error())
					return
				}
			}
		}
		_robotmap = make(map[int]robotCount) //	清空map
	}
	_activePool[instanceid] = time.Now().Unix() //更新活跃状态
	c.JSON(http.StatusOK, gin.H{
		"ret": 0,
		"msg": "success",
	})
}

//TransResult 统计事务
func TransResult(c *gin.Context) {

	tick, _ := strconv.Atoi(c.Query("timestamp"))
	if tick <= 1000 {
		c.JSON(http.StatusOK, gin.H{
			"ret": 1, //解析错误
			"msg": "success",
		})
	}
	instanceid, _ := strconv.Atoi(c.Query("instanceid"))
	type data struct {
		Data []transInfo `json:"data"`
	}

	var recv data
	_ = c.BindJSON(&recv)

	for i := 0; i < len(recv.Data); i++ {
		_, err = db.Exec(
			"insert into `trans_info`  (`timetick`,`name`,`result`,`cost`,`assigninstanceid`) values(?,?,?,?,?)", tick, recv.Data[i].TransName, recv.Data[i].TransResult, recv.Data[i].TimeCost, instanceid)
		if err != nil {
			fmt.Printf("data insert faied, error:[%v]", err.Error())
			return
		}
	}

	/*
		_transMap[tick] = make(map[transPair]transValue)
		for i := 0; i < len(recv.Data); i++ {
			var pair transPair
			pair.name = recv.Data[i].TransName
			pair.result = recv.Data[i].TransResult

			var value transValue
			value.cost = _transMap[tick][pair].cost
			value.cost += recv.Data[i].TimeCost
			value.num = _transMap[tick][pair].num
			value.num++
			_transMap[tick][pair] = value
		}

		if len(_transMap) > 8 {
			transmutex.Lock()
			defer transmutex.Unlock()
			for key, val := range _transMap {
				if key%2 == 0 {
					for pairkey, v := range val {
						_, err = db.Exec(
							"insert into `trans_info`  (`timetick`,`name`,`result`,`cost`,`num`,`assigninstanceid`) values(?,?,?,?,?,?)", key, pairkey.name, pairkey.result, v.cost, v.num, _assignInstanceID)
						if err != nil {
							fmt.Printf("data insert faied, error:[%v]", err.Error())
							return
						}
					}
				}
			}
			_transMap = make(map[int]map[transPair]transValue) //	清空map
		}
	*/

	c.JSON(http.StatusOK, gin.H{
		"ret": 0,
		"msg": "success",
	})
}

//CleanAllData 删除所有数据
func CleanAllData(c *gin.Context) {
	_, _ = db.Query("truncate table cpu_infomation")
	_, _ = db.Query("truncate table memory_infomation")
	_, _ = db.Query("truncate table net_flow")
	_, _ = db.Query("truncate table robot_info")
	_, _ = db.Query("truncate table trans_info")
	_, _ = db.Query("truncate table test_info")
	_, _ = db.Query("truncate table target_ip")
	_, _ = db.Query("truncate table plan_info")
}

//ActiveCheck 检查活跃的机器人
func ActiveCheck() {
	ticker := time.NewTicker(time.Second * 1) // 运行时长
	ch := make(chan int)
	go func() {
		for {
			select {
			case <-ticker.C:
				timeUnix := time.Now().Unix()
				//fmt.Println(timeUnix)
				for key, val := range _activePool {
					//有机器人连接超时
					if timeUnix-val >= 10 {
						endTest(key)
						delete(_activePool, key)
					}
				}
			}
		}
	}()
	<-ch // 通过通道阻塞，让任务可以执行完指定的次数。
}

func endTest(instanceid int) {
	_robotOnlineNum = 0
	_robotTotalNum = 0

	timeUnix := time.Now().Unix()
	//找到sid
	var sid int
	sql := fmt.Sprintf("select sid from test_info where id = %d ", instanceid)
	//fmt.Println(sql)
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&sid)
	}
	//关掉sid对应的所有服务器的monitor
	var monitorip string
	sql = fmt.Sprintf("select ip from target_ip where id = %d ", sid)
	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&monitorip)
		fmt.Println(monitorip)
		var user string
		var pwd string
		var monitorport int
		sql = fmt.Sprintf("select port,user,pwd from server_info where ip = \"%s\"", monitorip)
		rows, err := db.Query(sql)
		if err != nil {
			fmt.Printf("error: %v \n", err)
			return
		}
		for rows.Next() {
			rows.Scan(&monitorport, &user, &pwd)
			sesstion, err := SSHConnect(user, pwd, monitorip, monitorport)
			defer sesstion.Close()
			//fmt.Println(user, pwd, monitorip, monitorport)
			if err != nil {
				fmt.Printf("error: %v", err)
			}
			sesstion.Run("pkill monitor") //关闭监控
			fmt.Println(monitorip, "close over")
		}
	}
	//跟新状态表
	sql = fmt.Sprintf("update `test_info` set endtick = %d,status = %d where id = %d", timeUnix, TESTOVER, instanceid)
	//fmt.Println(sql)
	_, err = db.Exec(sql)
	if err != nil {
		fmt.Printf("error:[%v]", err.Error())
		return
	}

	go updateTransResult(instanceid)
	go updateTransRecord(instanceid)
}

func updateTransResult(instid int) {

	fmt.Println("2219 in updateTransResult")
	type ReturnDatas struct {
		Name        string  `json:"name"`
		Sum         int     `json:"sum"`
		Success     int     `json:"success"`
		Failed      int     `json:"failed"`
		Error       int     `json:"error"`
		TimeOut     int     `json:"timeout"`
		SuccessRate float32 `json:"successrate"`
		Avg         float32 `json:"tps"`
		MaxCost     int     `json:"maxcost"`
		MinCost     int     `json:"mincost"`
		Percent50   int     `json:"50percent"`
		Percent75   int     `json:"75percent"`
		Percent90   int     `json:"90percent"`
	}

	rows, err := db.Query("select count(*) from (select name,result as num from trans_info group by result,name,assigninstanceid having assigninstanceid = ?) as A", instid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	var size int
	for rows.Next() {
		rows.Scan(&size)
	}
	returnDatas := make([]ReturnDatas, 0, size)
	res := make(map[string]*ReturnDatas)
	percents := make(map[string]int)

	times := 0
	rows, err = db.Query("select count(*) from (select timetick from trans_info group by timetick,assigninstanceid having assigninstanceid = ?) as A;", instid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&times)
	}

	fmt.Printf("in updateTransResult times:%d", times)
	//2s
	rows, err = db.Query("select name,result,count(result) as num from trans_info group by result,name,assigninstanceid having assigninstanceid = ?", instid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var name string
		var result int
		var num int
		rows.Scan(&name, &result, &num)
		if res[name] == nil {
			res[name] = new(ReturnDatas)
			percents[name] = 0
		}
		if result == TRANSFAIL {
			res[name].Failed = num
		}
		if result == TRANSSUCCESS {
			res[name].Success = num
		}
		if result == TRANSOVERTIME {
			res[name].TimeOut = num
		}
		if result == TRANSERROR {
			res[name].Error = num
		}
	}

	rows, err = db.Query("select name,max(A.m) as max from (select name,max(cost) as m from trans_info group by name,assigninstanceid having assigninstanceid = ?) as A group by A.name", instid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var name string
		var max int
		rows.Scan(&name, &max)
		res[name].MaxCost = max
	}

	rows, err = db.Query("select name,min(A.m) as min from (select name,min(cost) as m from trans_info group by name,assigninstanceid having assigninstanceid = ?) as A group by A.name", instid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var name string
		var min int
		rows.Scan(&name, &min)
		res[name].MinCost = min
	}

	//2s
	rows, err = db.Query("select * from (select name,cost,count(cost) as num from trans_info group by name,cost,assigninstanceid having assigninstanceid = ?) as A  order by A.cost", instid)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var name string
		var cost int
		var num int
		rows.Scan(&name, &cost, &num)
		res[name].Sum = res[name].Success + res[name].Error + res[name].Failed + res[name].TimeOut
		res[name].Avg = float32(res[name].Sum) / float32(times)
		percents[name] += num
		if float32(percents[name]) >= float32(res[name].Sum)*0.5 && float32(percents[name]) < float32(res[name].Sum)*0.75 {
			(*res[name]).Percent50 = cost
		}
		if float32(percents[name]) >= float32(res[name].Sum)*0.75 && float32(percents[name]) < float32(res[name].Sum)*0.9 {
			(*res[name]).Percent75 = cost
		}
		if float32(percents[name]) >= float32(res[name].Sum)*0.9 {
			(*res[name]).Percent90 = cost
		}
	}

	for key, val := range res {
		var tmp ReturnDatas
		if val.Success == val.Success+val.Error+val.Failed+val.TimeOut && val.Success == 0 {
			val.SuccessRate = 100
		} else {
			val.SuccessRate = 100 * float32(val.Success) / float32(val.Success+val.Error+val.Failed+val.TimeOut)
		}
		tmp = *val
		tmp.Name = key
		returnDatas = append(returnDatas, tmp)
	}

	fmt.Printf("in updateTransResult returnDatas:%v\n", returnDatas)

	for i := 0; i < len(returnDatas); i++ {

		fmt.Println(len(returnDatas), returnDatas[i])
		sql := fmt.Sprintf("insert into `trans_result` (`instanceid`,`sum`,`success`,`failed`,`timeout`,`error`,`successrate`,`agv`,`maxcost`,`mincost`,`percent50`,`percent75`,`percent90`,`name`) values(%d,%d,%d,%d,%d,%d,%f,%f,%d,%d,%d,%d,%d,\"%s\")",
			instid, returnDatas[i].Sum, returnDatas[i].Success, returnDatas[i].Failed, returnDatas[i].TimeOut, returnDatas[i].Error, returnDatas[i].SuccessRate, returnDatas[i].Avg,
			returnDatas[i].MaxCost, returnDatas[i].MinCost, returnDatas[i].Percent50, returnDatas[i].Percent75, returnDatas[i].Percent90, returnDatas[i].Name)

		_, err = db.Exec(sql)

		if err != nil {
			fmt.Printf("data insert faied;2132 error:[%v]", err.Error())
			return
		}
	}
}

func updateTransRecord(instaceid int) {

	fmt.Println("in Record sid = ", instaceid)
	var size int
	sql := fmt.Sprintf("select count(*) from (select timetick  from trans_info group by timetick,name,assigninstanceid having assigninstanceid = %d order by timetick) as A;", instaceid)
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		rows.Scan(&size)
	}
	sql = fmt.Sprintf("select name,timetick,count(*) from trans_info group by timetick,name,assigninstanceid having assigninstanceid = %d order by timetick;", instaceid)

	rows, err = db.Query(sql)
	if err != nil {
		fmt.Printf("error: %v \n", err)
		return
	}
	for rows.Next() {
		var num int
		var tick int
		var name string
		rows.Scan(&name, &tick, &num)
		_, err := db.Exec(
			"insert into `trans_record_result`  (`instanceid`,`tick`,`nums`,`name`) values(?,?,?,?)", instaceid, tick, num, name)
		if err != nil {
			fmt.Printf("data insert faied, error:[%v]", err.Error())
			return
		}
	}
}
