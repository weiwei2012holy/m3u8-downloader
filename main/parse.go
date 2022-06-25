package main

import (
    "context"
    "flag"
    "fmt"
    "github.com/chromedp/cdproto/cdp"
    "github.com/chromedp/chromedp"
    "github.com/robertkrimen/otto"
    "log"
    "m3u8-downloader/logic"
    "time"
)

func main() {

    url := flag.String("u", "", "需要解析的网址")
    flag.Parse()
    if *url == "" {
        log.Fatal("无效的网址")
    }

    var js, infoNodes []*cdp.Node
    var title, title2 string
    //var tsLink string
    //var buf []byte

    opts := append(chromedp.DefaultExecAllocatorOptions[:],
        //chromedp.DisableGPU,
        chromedp.UserAgent(`Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.80 Safari/537.36`),
    )

    allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
    defer cancel()
    ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
    defer cancel()
    //超时控制
    timeoutCtx, cancel := context.WithTimeout(ctx, time.Second*25)
    defer cancel()

    err := chromedp.Run(
        timeoutCtx,
        chromedp.Navigate(*url),
        chromedp.WaitVisible(`section.video-info.pb-3`),
        //chromedp.Sleep(time.Second*5),
        //chromedp.Nodes(`#site-content > div > div > div:nth-child(1) > section.video-info.pb-3 > div.info-header > div.header-left > h4`, &nodes),
        //chromedp.FullScreenshot(&buf, 100),
        chromedp.Title(&title),
        chromedp.Nodes(`#site-content > div > div > div:nth-child(1) > section.pb-3.pb-e-lg-30 > script:nth-child(3)`, &js),

        chromedp.Nodes(`#site-content > div > div > div:nth-child(1) > section.video-info.pb-3 > div.info-header > div.header-left > h4`, &infoNodes),
        //chromedp.JavascriptAttribute(
        //    `#site-content > div > div > div:nth-child(1) > section.pb-3.pb-e-lg-30 > script:nth-child(3)`,
        //    "hlsUrl",
        //    &tsLink,
        //),
    )
    if err != nil {
        log.Fatal(err)
    }

    title2 = infoNodes[0].Children[0].NodeValue
    if title2 != "" {
        title = title2
    }

    vm := otto.New()
    if len(js) == 0 || js[0].ChildNodeCount == 0 {
        log.Fatal("解析HTML中JS脚本失败")
    }
    jsString := js[0].Children[0].NodeValue
    vm.Run(jsString)

    link, err := vm.Get("hlsUrl")
    if err != nil {
        log.Fatal("获取链接失败:", err)
    }
    video := link.String()

    dir := "movie"
    hostType := "apiv3"

    arg := logic.InputArguments{
        FlagUrl: &video,
        FlagHT:  &hostType,
        FlagO:   &title,
        FlagSP:  &dir,
    }
    fmt.Println("解析成功，开始下载", title, link)

    logic.RunLogic(arg)

}
