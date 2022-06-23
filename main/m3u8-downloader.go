package main

import (
    "flag"
    "m3u8-downloader/logic"
)

var (
    inputArguments = logic.InputArguments{
        FlagUrl: flag.String("u", "", "m3u8下载地址(http(s)://url/xx/xx/index.m3u8)"),
        FlagN:   flag.Int("n", 16, "下载线程数(max goroutines num)"),
        FlagHT:  flag.String("ht", "apiv1", "设置getHost的方式(apiv1: `http(s):// + url.Host + filepath.Dir(url.Path)`; apiv2: `http(s)://+ u.Host`"),
        FlagO:   flag.String("o", "movie", "自定义文件名(默认为movie)"),
        FlagC:   flag.String("c", "", "自定义请求 cookie"),
        FlagS:   flag.Int("s", 0, "是否允许不安全的请求(默认为0)"),
        FlagSP:  flag.String("sp", "", "文件保存路径(默认为当前路径)"),
    }
)

func main() {
    // 解析命令行参数
    flag.Parse()
    logic.RunLogic(inputArguments)
}
