//@author:llychao<lychao_vip@163.com>
//@contributor: Junyi<me@junyi.pw>
//@date:2020-02-18
//@功能:golang m3u8 video Downloader
package logic

import (
    "bytes"
    "crypto/aes"
    "crypto/cipher"
    "errors"
    "flag"
    "fmt"
    "github.com/levigross/grequests"
    "github.com/yapingcat/gomedia/mp4"
    "github.com/yapingcat/gomedia/mpeg2"
    "io/ioutil"
    "log"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strconv"
    "strings"
    "sync"
    "time"
)

const (
    // HeadTimeout 请求头超时时间
    HeadTimeout = 10 * time.Second
    // ProgressWidth 进度条长度
    ProgressWidth = 20
    // TsNameTemplate ts视频片段命名规则
    TsNameTemplate = "%05d.ts"

    TempTsFileName   = "merge.tmp"
    TransTmpFileName = "merge.mp4"
)

type InputArguments struct {
    FlagUrl *string
    FlagN   *int
    FlagHT  *string
    FlagO   *string
    FlagC   *string
    FlagS   *int
    FlagSP  *string
}

// TsInfo 用于保存 ts 文件的下载地址和文件名
type TsInfo struct {
    Name string
    Url  string
}

func init() {
    logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
}

var (
    logger *log.Logger
    ro     = &grequests.RequestOptions{
        UserAgent:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/79.0.3945.88 Safari/537.36",
        RequestTimeout: HeadTimeout,
        Headers: map[string]string{
            "Connection":      "keep-alive",
            "Accept":          "*/*",
            "Accept-Encoding": "*",
            "Accept-Language": "zh-CN,zh;q=0.9, en;q=0.8, de;q=0.7, *;q=0.5",
        },
    }
)

func RunLogic(inputArguments InputArguments) {
    msgTpl := "[功能]:多线程下载直播流 m3u8 视屏（ts + 合并）\n[提醒]:如果下载失败，请使用 -ht=apiv2 \n[提醒]:如果下载失败，m3u8 地址可能存在嵌套\n[提醒]:如果进度条中途下载失败，可重复执行"
    fmt.Println(msgTpl)
    runtime.GOMAXPROCS(runtime.NumCPU())
    now := time.Now()
    m3u8Url := *inputArguments.FlagUrl
    maxGoroutines := *inputArguments.FlagN
    hostType := *inputArguments.FlagHT
    movieDir := *inputArguments.FlagO
    cookie := *inputArguments.FlagC
    insecure := *inputArguments.FlagS
    savePath := *inputArguments.FlagSP

    ro.Headers["Referer"] = getHost(m3u8Url, "apiv2")
    if insecure != 0 {
        ro.InsecureSkipVerify = true
    }
    // http 自定义 cookie
    if cookie != "" {
        ro.Headers["Cookie"] = cookie
    }
    if !strings.HasPrefix(m3u8Url, "http") || m3u8Url == "" {
        flag.Usage()
        return
    }
    var downloadDir string
    pwd, _ := os.Getwd()
    if savePath != "" {
        if strings.HasPrefix(savePath, "/") {
            pwd = savePath
        } else {
            pwd += "/" + savePath
        }
    }
    //pwd = "/Users/chao/Desktop" //自定义地址
    downloadDir = filepath.Join(pwd, movieDir)
    if isExist, _ := pathExists(downloadDir); !isExist {
        os.MkdirAll(downloadDir, os.ModePerm)
    }
    m3u8Host := getHost(m3u8Url, hostType)
    m3u8Body := getM3u8Body(m3u8Url)
    //m3u8Body := getFromFile()
    tsKey := getM3u8Key(m3u8Host, m3u8Body)
    if tsKey != "" {
        fmt.Printf("待解密 ts 文件 key : %s \n", tsKey)
    }
    tsList := getTsList(m3u8Host, m3u8Body)

    fmt.Println("待下载 ts 文件数量:", len(tsList))
    // 下载ts
    downloader(tsList, maxGoroutines, downloadDir, tsKey)

    if ok := checkTsDownDir(downloadDir); !ok {
        fmt.Printf("\n[Failed] 请检查url地址有效性 \n")
        return
    }

    err := videMergeToMp4(tsList, downloadDir, movieDir)
    if err != nil {
        log.Fatal("格式转换失败", err)
    }

    //err = os.Rename(filepath.Join(downloadDir, "merge.mp4"), downloadDir+".mp4")
    //if err != nil {
    //    log.Fatal("重命名失败:", err)
    //}
    err = os.RemoveAll(downloadDir)
    if err != nil {
        log.Fatal("删除下载文件失败:", err)
    }
    fmt.Printf("\n[Success] 下载保存路径：%s | 共耗时: %6.2fs\n", downloadDir+".mp4", time.Now().Sub(now).Seconds())
}

func videMergeToMp4(tsList []TsInfo, path, fileName string) error {
    if fileName == "" {
        fileName = TransTmpFileName
    }
    fileName += ".mp4"

    err := os.Chdir(path)
    if err != nil {
        return err
    }

    mp4file, err := os.OpenFile("../"+fileName, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
    if err != nil {
        return err
    }
    defer mp4file.Close()

    muxer := mp4.CreateMp4Muxer(mp4file)

    vtid := muxer.AddVideoTrack(mp4.MP4_CODEC_H264)
    atid := muxer.AddAudioTrack(mp4.MP4_CODEC_AAC, 0, 16, 44100)
    demuxer := mpeg2.NewTSDemuxer()
    var OnFrameErr error
    demuxer.OnFrame = func(cid mpeg2.TS_STREAM_TYPE, frame []byte, pts uint64, dts uint64) {
        if OnFrameErr != nil {
            return
        }
        if cid == mpeg2.TS_STREAM_AAC {
            OnFrameErr = muxer.Write(atid, frame, pts, dts)
        } else if cid == mpeg2.TS_STREAM_H264 {
            OnFrameErr = muxer.Write(vtid, frame, pts, dts)
        } else {
            OnFrameErr = errors.New("unknown cid " + strconv.Itoa(int(cid)))
        }
    }

    for i, ts := range tsList {
        buf, err := ioutil.ReadFile(ts.Name)
        if err != nil {
            return err
        }
        err = demuxer.Input(bytes.NewReader(buf))
        if err != nil {
            return err
        }
        if OnFrameErr != nil {
            return OnFrameErr
        }

        DrawProgressBar("转换中", float32(i+1)/float32(len(tsList)), ProgressWidth, ts.Name)

    }

    err = muxer.WriteTrailer()
    if err != nil {
        return err
    }
    err = mp4file.Sync()
    if err != nil {
        return err
    }

    execUnixShell("rm -rf *.ts")

    return nil
}

// 获取m3u8地址的host
func getHost(Url, ht string) (host string) {
    u, err := url.Parse(Url)
    checkErr(err)
    switch ht {
    case "apiv1":
        host = u.Scheme + "://" + u.Host + filepath.Dir(u.EscapedPath())
    case "apiv2":
        host = u.Scheme + "://" + u.Host
    }
    return
}

// 获取m3u8地址的内容体
func getM3u8Body(Url string) string {
    r, err := grequests.Get(Url, ro)
    checkErr(err)
    return r.String()
}

// 获取m3u8加密的密钥
func getM3u8Key(host, html string) (key string) {
    lines := strings.Split(html, "\n")
    key = ""
    for _, line := range lines {
        if strings.Contains(line, "#EXT-X-KEY") {
            uri_pos := strings.Index(line, "URI")
            quotation_mark_pos := strings.LastIndex(line, "\"")
            key_url := strings.Split(line[uri_pos:quotation_mark_pos], "\"")[1]
            if !strings.Contains(line, "http") {
                key_url = fmt.Sprintf("%s/%s", host, key_url)
            }
            res, err := grequests.Get(key_url, ro)
            checkErr(err)
            if res.StatusCode == 200 {
                key = res.String()
            }
        }
    }
    return
}

func getTsList(host, body string) (tsList []TsInfo) {
    lines := strings.Split(body, "\n")
    index := 0
    var ts TsInfo
    for _, line := range lines {
        if !strings.HasPrefix(line, "#") && line != "" {
            //有可能出现的二级嵌套格式的m3u8,请自行转换！
            index++
            if strings.HasPrefix(line, "http") {
                ts = TsInfo{
                    Name: fmt.Sprintf(TsNameTemplate, index),
                    Url:  line,
                }
                tsList = append(tsList, ts)
            } else {
                ts = TsInfo{
                    Name: fmt.Sprintf(TsNameTemplate, index),
                    Url:  fmt.Sprintf("%s/%s", host, line),
                }
                tsList = append(tsList, ts)
            }
        }
    }
    return
}

func getFromFile() string {
    data, _ := ioutil.ReadFile("./ts.txt")
    return string(data)
}

// 下载ts文件
// @modify: 2020-08-13 修复ts格式SyncByte合并不能播放问题
func downloadTsFile(ts TsInfo, download_dir, key string, retries int) {
    defer func() {
        if r := recover(); r != nil {
            //fmt.Println("网络不稳定，正在进行断点持续下载")
            downloadTsFile(ts, download_dir, key, retries-1)
        }
    }()
    curr_path := fmt.Sprintf("%s/%s", download_dir, ts.Name)
    if isExist, _ := pathExists(curr_path); isExist {
        //logger.Println("[warn] File: " + ts.Name + "already exist")
        return
    }
    res, err := grequests.Get(ts.Url, ro)
    if err != nil || !res.Ok {
        if retries > 0 {
            downloadTsFile(ts, download_dir, key, retries-1)
            return
        } else {
            //logger.Printf("[warn] File :%s", ts.Url)
            return
        }
    }
    // 校验长度是否合法
    var origData []byte
    origData = res.Bytes()
    contentLen := 0
    contentLenStr := res.Header.Get("Content-Length")
    if contentLenStr != "" {
        contentLen, _ = strconv.Atoi(contentLenStr)
    }
    if len(origData) == 0 || (contentLen > 0 && len(origData) < contentLen) || res.Error != nil {
        //logger.Println("[warn] File: " + ts.Name + "res origData invalid or err：", res.Error)
        downloadTsFile(ts, download_dir, key, retries-1)
        return
    }
    // 解密出视频 ts 源文件
    if key != "" {
        //解密 ts 文件，算法：aes 128 cbc pack5
        origData, err = AesDecrypt(origData, []byte(key))
        if err != nil {
            downloadTsFile(ts, download_dir, key, retries-1)
            return
        }
    }
    // https://en.wikipedia.org/wiki/MPEG_transport_stream
    // Some TS files do not start with SyncByte 0x47, they can not be played after merging,
    // Need to remove the bytes before the SyncByte 0x47(71).
    syncByte := uint8(71) //0x47
    bLen := len(origData)
    for j := 0; j < bLen; j++ {
        if origData[j] == syncByte {
            origData = origData[j:]
            break
        }
    }
    ioutil.WriteFile(curr_path, origData, 0666)
}

// downloader m3u8 下载器
func downloader(tsList []TsInfo, maxGoroutines int, downloadDir string, key string) {
    retry := 5 //单个 ts 下载重试次数
    var wg sync.WaitGroup
    limiter := make(chan struct{}, maxGoroutines) //chan struct 内存占用 0 bool 占用 1
    tsLen := len(tsList)
    downloadCount := 0
    for _, ts := range tsList {
        wg.Add(1)
        limiter <- struct{}{}
        go func(ts TsInfo, downloadDir, key string, retryies int) {
            defer func() {
                wg.Done()
                <-limiter
            }()
            downloadTsFile(ts, downloadDir, key, retryies)
            downloadCount++
            DrawProgressBar("Downloading", float32(downloadCount)/float32(tsLen), ProgressWidth, ts.Name)
            return
        }(ts, downloadDir, key, retry)
    }
    wg.Wait()
}

func checkTsDownDir(dir string) bool {
    if isExist, _ := pathExists(filepath.Join(dir, fmt.Sprintf(TsNameTemplate, 0))); !isExist {
        return true
    }
    return false
}

// 进度条
func DrawProgressBar(prefix string, proportion float32, width int, suffix ...string) {
    pos := int(proportion * float32(width))
    s := fmt.Sprintf("[%s] %s%*s %6.2f%% \t%s",
        prefix, strings.Repeat("■", pos), width-pos, "", proportion*100, strings.Join(suffix, ""))
    fmt.Print("\r" + s)
}

// ============================== shell相关 ==============================
// 判断文件是否存在
func pathExists(path string) (bool, error) {
    _, err := os.Stat(path)
    if err == nil {
        return true, nil
    }
    if os.IsNotExist(err) {
        return false, nil
    }
    return false, err
}

// 执行 shell
func execUnixShell(s string) {
    cmd := exec.Command("/bin/bash", "-c", s)
    var out bytes.Buffer
    cmd.Stdout = &out
    err := cmd.Run()
    if err != nil {
        panic(err)
    }
    fmt.Printf("%s", out.String())
}

func execWinShell(s string) error {
    cmd := exec.Command("cmd", "/C", s)
    var out bytes.Buffer
    cmd.Stdout = &out
    err := cmd.Run()
    if err != nil {
        return err
    }
    fmt.Printf("%s", out.String())
    return nil
}

// windows 合并文件
func win_merge_file(path string) {
    os.Chdir(path)
    execWinShell(fmt.Sprintf("copy /b *.ts %s", TempTsFileName))
    execWinShell("del /Q *.ts")
    videTrans(TempTsFileName, TransTmpFileName)
}

// unix 合并文件
func unix_merge_file(path string) {
    os.Chdir(path)
    //cmd := `ls  *.ts |sort -t "\." -k 1 -n |awk '{print $0}' |xargs -n 1 -I {} bash -c "cat {} >> new.tmp"`
    cmd := fmt.Sprintf(`cat *.ts >> %s`, TempTsFileName)
    execUnixShell(cmd)
    execUnixShell("rm -rf *.ts")

    //格式转换
    videTrans(TempTsFileName, TransTmpFileName)
}

func videTrans(input, outPut string) {
    //ffmpeg 是否安装
    //cmd := fmt.Sprintf(`ffmpeg -i %s %s`, TempTsFileName, TransTmpFileName)
    //fmt.Println(cmd)
    //execUnixShell(cmd)

    err := os.Rename(input, outPut)
    if err != nil {
        log.Fatal("格式转换失败", err)
    }
}

// ============================== 加解密相关 ==============================

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
    padding := blockSize - len(ciphertext)%blockSize
    padtext := bytes.Repeat([]byte{byte(padding)}, padding)
    return append(ciphertext, padtext...)
}

func PKCS7UnPadding(origData []byte) []byte {
    length := len(origData)
    unpadding := int(origData[length-1])
    return origData[:(length - unpadding)]
}

func AesEncrypt(origData, key []byte, ivs ...[]byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    blockSize := block.BlockSize()
    var iv []byte
    if len(ivs) == 0 {
        iv = key
    } else {
        iv = ivs[0]
    }
    origData = PKCS7Padding(origData, blockSize)
    blockMode := cipher.NewCBCEncrypter(block, iv[:blockSize])
    crypted := make([]byte, len(origData))
    blockMode.CryptBlocks(crypted, origData)
    return crypted, nil
}

func AesDecrypt(crypted, key []byte, ivs ...[]byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    blockSize := block.BlockSize()
    var iv []byte
    if len(ivs) == 0 {
        iv = key
    } else {
        iv = ivs[0]
    }
    blockMode := cipher.NewCBCDecrypter(block, iv[:blockSize])
    origData := make([]byte, len(crypted))
    blockMode.CryptBlocks(origData, crypted)
    origData = PKCS7UnPadding(origData)
    return origData, nil
}

func checkErr(e error) {
    if e != nil {
        logger.Panic(e)
    }
}
