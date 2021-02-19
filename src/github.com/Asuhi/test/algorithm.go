package main

import (
	"fmt"
	"math/big"
	"net"
)

//AvgInt 平均值
func AvgInt(values []int64) float64 {
	var sum int64
	len := len(values)
	for i := 0; i < len; i++ {
		sum += values[i]
	}
	return float64(sum) / float64(len)
}

//AvgFloat32 平均值
func AvgFloat32(values []float32) float32 {
	var sum float32
	len := len(values)
	for i := 0; i < len; i++ {
		sum += values[i]
	}
	return sum / float32(len)
}

//AvgFloat64 平均值
func AvgFloat64(values []float64) float64 {
	var sum float64
	len := len(values)
	for i := 0; i < len; i++ {
		sum += values[i]
	}
	return sum / float64(len)
}

//LinearRegressionInt 线性回归f(x) = a+bx
func LinearRegressionInt(x []int64, y []float64) (float64, float64) {
	xavg := AvgInt(x)
	yavg := AvgFloat64(y)
	var xm float64
	var ym float64
	for i := 0; i < len(x); i++ {
		ym += (float64(x[i]) - xavg) * (y[i] - yavg)
		xm += (float64(x[i]) - xavg) * (float64(x[i]) - xavg)
	}
	if xm == 0 {
		return 0, 0
	}
	b := ym / xm
	a := yavg - b*xavg
	return a, b
}

//InetNtoA 数字转字符串IP
func InetNtoA(ip int64) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

//InetAtoN 字符串IP转数字
func InetAtoN(ip string) int64 {
	ret := big.NewInt(0)
	ret.SetBytes(net.ParseIP(ip).To4())
	return ret.Int64()
}
