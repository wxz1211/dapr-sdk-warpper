package dapr_sdk_warpper

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestTimeJSON(t *testing.T) {
	type test struct {
		CreateAt JSONTime
	}
	data, err := json.Marshal(&test{
		CreateAt: JSONTime(time.Now()),
	})
	if err != nil {
		t.Fatalf("%v\n", err)
	}
	t.Logf("%s\n", data)

	tt := test{}
	json.Unmarshal(data, &tt)
	t.Logf("%v\n", time.Time(tt.CreateAt).String())
}

func TestHgMicro(t *testing.T) {
	var ctx context.Context
	t.Logf("%v", reflect.TypeOf(&ctx).Elem().String())
}

type Permission struct {
	//这里未来可以和调用权限管理挂起来
	ServiceID string   `json:"service_id"`
	Method    []string `json:"method"`
}

type SubDemo struct {
	ID    int     `json:"id,omitempty"`
	Float float64 `json:"float"`
	Bool  bool    `json:"boolean"`
}

type YamlDemo struct {
	SubDemo
	Name       string            `json:"name"`
	Meta       map[string]string `json:"meta"`
	Permission []*Permission     `json:"permission"`
}

type BaseResponse struct {
	Result  int    `json:"result"`
	Message string `json:"msg"`
}

//-------------------获取登录类型-------------------

// LoginKind 登录类型
type LoginKind struct {
	Name  string `json:"name"`  //类型名称，一般不作为显示用
	Label string `json:"label"` //类型名称，可显示在UI中的名字
}

// GetLoginKindRequest 获取登录方式
type GetLoginKindRequest struct {
	Channel string `json:"channel" binding:"required,oneof=web app"` //鉴权（登录）通道web,app
}

// GetLoginKindResponse 获取登录类型
type GetLoginKindResponse struct {
	BaseResponse
	Kinds    []LoginKind `json:"kinds"`
	CreateAt time.Time   `json:"create"`
}

type AreaNode struct {
	Code     string      `json:"code"`
	Name     string      `json:"name"`
	Children []*AreaNode `json:"children"`
}

type DemoServer struct {
}

func (s *DemoServer) GetLoginKind(ctx context.Context, in *AreaNode, out *GetLoginKindResponse) error {
	return nil
}

func TestRegister(t *testing.T) {
	s := &DemoServer{}
	defaultDaprServer := newDaprServer()
	if err := defaultDaprServer.registMethods("className", s); err != nil {
		t.Log(err)
	}
	t.Log(defaultDaprServer.getSignatureYaml())
}

func TestNameFormat(t *testing.T) {
	ret, err := structToYaml(&AreaNode{})
	if err != nil {
		t.Fatalf("\n%v", err)
	}
	signature := map[string][]refFieldInfo{"in": ret}
	data, err := yaml.Marshal(signature)
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Logf("%s\n", string(data))
}
