package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"github.com/abadojack/whatlanggo"
	"github.com/avast/retry-go"
	"github.com/briandowns/spinner"
	"github.com/goware/urlx"
	"github.com/olekukonko/tablewriter"
	"github.com/tidwall/gjson"
	"github.com/xiaoxuan6/deeplx"
	"io/ioutil"
	"mvdan.cc/xurls/v2"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	repository string

	green  = "\033[32m%s\033[0m\n"
	red    = "\033[31m%s\033[0m\n"
	yellow = "\033[33m%s\033[0m"
)

func main() {
	flag.StringVar(&repository, "repo", "", "repository")
	flag.Parse()

SCANS:
	if len(repository) == 0 {
	SCAN:
		fmt.Printf(yellow, "请输入有效的 github repository：")
		_, _ = fmt.Scanln(&repository)

		if len(repository) == 0 {
			goto SCAN
		}
	}

	// 从字符串中提取 repository url
	rxRelaxed := xurls.Relaxed()
	domain := rxRelaxed.FindString(repository)
	if len(domain) == 0 {
		repository = ""
		goto SCANS
	}

	// 解析 url
	url, err := urlx.Parse(domain)
	if err != nil {
		fmt.Printf(red, "无效的 repository ["+domain+"]")
		repository = ""
		goto SCANS
	}

	// 格式化 url
	normalized, _ := urlx.Normalize(url)
	if !strings.HasPrefix(normalized, "http://github.com") && !strings.HasPrefix(normalized, "https://github.com") {
		fmt.Printf(red, "无效的 repository, 示例：https://githb.com/xiaoxuan6/gmod")
		repository = ""
		goto SCANS
	}

	uri := strings.ReplaceAll(normalized, "https", "http")
	uri = strings.ReplaceAll(uri, "http", "https")
	u, _ := urlx.Parse(uri)

	var modPath string
	paths := strings.Split(u.Path, "/")
	if len(paths) == 3 || strings.HasPrefix(uri, "https://github.com") {
		body := httpRetry(fmt.Sprintf("https://ungh.xiaoxuan6.me/repos/%s/%s", paths[1], paths[2]))
		language := strings.ToLower(gjson.GetBytes(body, "repo.language").String())
		if strings.Compare(language, "go") != 0 {
			fmt.Printf(red, "repository language is ["+language+"] not support go.mod")
			return
		}

		defaultBranch := gjson.GetBytes(body, "repo.defaultBranch").String()
		if len(paths) == 3 {
			modPath = fmt.Sprintf("%s/blob/%s/go.mod", uri, defaultBranch)
		} else {
			modPath = fmt.Sprintf("https://github.com/%s/%s/blob/%s/go.mod", paths[1], paths[2], defaultBranch)
		}
	} else if strings.HasSuffix(u.Path, "/go.mod") {
		modPath = uri
	} else {
		repository = ""
		fmt.Printf(red, "无效的 repository")
		goto SCANS
	}

	s := spinner.New(spinner.CharSets[36], 100*time.Millisecond)
	s.Start()

	var wgs sync.WaitGroup
	wgs.Add(2)
	go loadTemplate(&wgs)
	time.Sleep(1 * time.Second)
	go fetchModContent(modPath, &wgs)
	wgs.Wait()

	if len(templates) == 0 {
		time.Sleep(1 * time.Second)
		wgs.Add(1)
		go loadTemplate(&wgs)
		wgs.Wait()
	}

	newRepository := make([]string, 0)
	for _, val := range repositories {
		if !slices.Contains(templates, val) {
			newRepository = append(newRepository, val)
		}
	}

	for _, repos := range newRepository {
		wgs.Add(1)
		go translate(&wgs, repos)
	}
	wgs.Wait()

	s.Stop()

	if len(newRepository) == 0 {
		fmt.Printf(green, fmt.Sprintf("go.mod 中包含 %d 个 repository", len(repositories)))
		fmt.Printf(green, "【github.com/xiaoxuan6/go-package-example】已全覆盖 go.mod")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Repo", "Stars", "Forks", "Desc"})
	table.AppendBulk(tableData)
	table.SetRowLine(true)
	table.Render()
}

var tableData = make([][]string, 0)

func translate(wg *sync.WaitGroup, url string) {
	defer wg.Done()

	path := strings.TrimPrefix(url, "github.com/")
	paths := strings.Split(path, "/")
	body := httpRetry(fmt.Sprintf("https://ungh.xiaoxuan6.me/repos/%s/%s", paths[0], paths[1]))

	repo := gjson.GetBytes(body, "repo.repo").String()
	description := gjson.GetBytes(body, "repo.description").String()
	stars := gjson.GetBytes(body, "repo.stars").String()
	forks := gjson.GetBytes(body, "repo.forks").String()

	if len(description) == 0 {
		return
	}

	lang := whatlanggo.DetectLang(description)
	sourceLang := strings.ToUpper(lang.Iso6391())
	if strings.Compare(sourceLang, "zh") != 0 {
		response := deeplx.Translate(description, sourceLang, "zh")
		if response.Code == 200 {
			description = response.Data
		} else {
			response = deeplx.TranslateWithProxyUrl(description, sourceLang, "zh", true)
			if response.Code == 200 {
				description = response.Data
			}
		}
	}

	tableData = append(tableData, []string{
		fmt.Sprintf("github.com/%s", repo), stars, forks, description,
	})
}

var templates = make([]string, 0)

func loadTemplate(wg *sync.WaitGroup) {
	defer wg.Done()
	body := httpRetry("https://github-mirror.xiaoxuan6.me/https://github.com/xiaoxuan6/go-package-example/blob/main/README.md")
	f := bufio.NewScanner(strings.NewReader(string(body)))
	for f.Scan() {
		line := strings.TrimSpace(f.Text())
		if strings.HasPrefix(line, "|") {
			sLine := strings.Split(line, "|")
			templates = append(templates, sLine[2])
		}
	}
}

var repositories = make([]string, 0)

func fetchModContent(url string, wg *sync.WaitGroup) {
	defer wg.Done()
	body := httpRetry(fmt.Sprintf("https://github-mirror.xiaoxuan6.me/%s", url))
	f := bufio.NewScanner(strings.NewReader(string(body)))
	for f.Scan() {
		newLine := strings.TrimSpace(f.Text())
		if strings.Contains(newLine, "module") ||
			strings.Contains(newLine, "indirect") ||
			len(newLine) == 0 {
			continue
		}

		if strings.HasPrefix(newLine, "github.com") {
			sLine := strings.Split(newLine, " ")
			repositories = append(repositories, sLine[0])
		}
	}
}

func httpRetry(url string) (body []byte) {
	err := retry.Do(
		func() error {
			client := http.Client{
				Timeout: 3 * time.Second,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			}

			res, err := client.Get(url)
			if err != nil {
				return err
			}
			defer res.Body.Close()

			body, _ = ioutil.ReadAll(res.Body)
			if len(string(body)) == 0 {
				return errors.New("请求结果为空！")
			}

			return nil
		},
		retry.Attempts(3),
		retry.LastErrorOnly(true),
	)

	if err != nil {
		fmt.Printf(red, "fetch repository error: "+err.Error())
		os.Exit(1)
	}

	return body
}
