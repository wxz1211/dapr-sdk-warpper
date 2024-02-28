package main

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/wxz1211/dapr-sdk-warpper/server"
)

// Echo 函数的入参
type EchoIn struct {
	Message  string    `json:"message"`
	CreateAt time.Time `json:"create_at"`
}

type Extention struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Echo 函数的出参
type EchoOut struct {
	Message    string       `json:"message"`
	Extentions []*Extention `json:"extention"`
	EchoAt     time.Time    `json:"echo_at"`
}

// Update Info函数不需要返回所有没有“Out”类的出参
type UpdateIn struct {
	IsNewUser   bool   `json:"is_new"`
	Age         int    `json:"age"`
	BodyHeights int    `json:"body_heights"`
	Name        string `json:"name"`
	LoginDate   int    `json:"login_date"`
}

// 定义一个服务主体
type EchoServer struct {
}

// 定义一个函数
func (e *EchoServer) Echo(ctx context.Context, in *EchoIn, out *EchoOut) error {
	out.Message = fmt.Sprintf("FYI: %s", in.Message)
	out.EchoAt = time.Now()
	return nil
}

// 定义一个函数，注意：out参数为interface{}时，表示该参数被忽略。也就是
// 函数不需要返回内容。主要依赖调用是否成功来判断“调用结果”
func (e *EchoServer) UpdateInfo(ctx context.Context, in *UpdateIn, out interface{}) error {
	return nil
}

func main() {

	svc, err := sdk.NewServiceWithDapr(":2000", sdk.GRPC, "demo.echo.hugelink.cn/v1", &EchoServer{})
	if err != nil {
		panic(err)
	}

	err = svc.Start()
	if err != nil {
		fmt.Printf("Service start failure with %v\n", err)
	}

}
