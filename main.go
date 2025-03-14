package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mr-tron/base58"
	"github.com/urfave/cli/v2"
)

// Result 用于保存找到的地址及对应的公私钥
type Result struct {
	Address    string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// privateKeyToJSON 将私钥转换为 JSON 数组格式，便于 Solana 导入
func privateKeyToJSON(privateKey ed25519.PrivateKey) (string, error) {
	ints := make([]int, len(privateKey))
	for i, b := range privateKey {
		ints[i] = int(b)
	}
	data, err := json.MarshalIndent(ints, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func main() {
	app := &cli.App{
		Name:  "solana-address-generator",
		Usage: "生成符合指定前缀或后缀的 Solana 地址，并输出对应的公私钥",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "prefix",
				Usage:   "期望地址的前缀",
				Aliases: []string{"p"},
			},
			&cli.StringFlag{
				Name:    "postfix",
				Usage:   "期望地址的后缀",
				Aliases: []string{"s"},
			},
			&cli.BoolFlag{
				Name:    "case-sensitive",
				Usage:   "是否开启大小写敏感匹配",
				Value:   true,
				Aliases: []string{"c"},
			},
		},
		Action: func(c *cli.Context) error {
			prefix := c.String("prefix")
			postfix := c.String("postfix")
			caseSensitive := c.Bool("case-sensitive")

			if prefix == "" && postfix == "" {
				return fmt.Errorf("必须至少指定 --prefix 或 --postfix 其中之一")
			}

			fmt.Println("开始生成 Solana 地址...")

			var totalIterations int64
			start := time.Now()

			// 使用 channel 接收第一个找到的结果
			resultCh := make(chan Result, 1)
			// 使用 context 控制 goroutine 退出
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var wg sync.WaitGroup
			numWorkers := runtime.NumCPU()

			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						// 检查是否已取消
						select {
						case <-ctx.Done():
							return
						default:
						}
						atomic.AddInt64(&totalIterations, 1)
						publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
						if err != nil {
							continue
						}
						// 生成 Solana 地址（Base58 编码公钥）
						address := base58.Encode(publicKey)

						// 根据大小写敏感参数处理匹配
						checkAddress := address
						checkPrefix := prefix
						checkPostfix := postfix
						if !caseSensitive {
							checkAddress = strings.ToLower(address)
							checkPrefix = strings.ToLower(prefix)
							checkPostfix = strings.ToLower(postfix)
						}

						if prefix != "" && !strings.HasPrefix(checkAddress, checkPrefix) {
							continue
						}
						if postfix != "" && !strings.HasSuffix(checkAddress, checkPostfix) {
							continue
						}

						// 找到符合条件的地址
						select {
						case resultCh <- Result{
							Address:    address,
							PublicKey:  publicKey,
							PrivateKey: privateKey,
						}:
							cancel() // 通知其它 goroutine 停止
							return
						case <-ctx.Done():
							return
						}
					}
				}()
			}

			// 等待第一个结果
			result := <-resultCh
			wg.Wait()
			elapsed := time.Since(start)

			fmt.Printf("找到地址: %s\n", result.Address)
			fmt.Printf("尝试次数: %d, 耗时: %s\n", totalIterations, elapsed)
			fmt.Printf("公钥 (Base58): %s\n", base58.Encode(result.PublicKey))
			fmt.Printf("私钥 (Base58): %s\n", base58.Encode(result.PrivateKey))
			// 将私钥转换为 JSON 数组格式输出
			pkJSON, err := privateKeyToJSON(result.PrivateKey)
			if err != nil {
				return fmt.Errorf("转换私钥为JSON失败: %v", err)
			}
			fmt.Printf("私钥 (JSON 数组格式): %s\n", pkJSON)
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

