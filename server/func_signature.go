package dapr_sdk_warpper

import (
	"context"
	"reflect"

	"github.com/dapr/go-sdk/service/common"
	"gopkg.in/yaml.v3"
)

type refMethodSignature struct {
	Name string         `yaml:"name"`
	In   []refFieldInfo `yaml:"in"`
	Out  []refFieldInfo `yaml:"out"`
}

type serviceSignature struct {
	APIVersion string                `yaml:"apiVersion"`
	Spec       []*refMethodSignature `yaml:"spec"`
}

//获取服务函数的签名
func (server *daprServer) getSignature() (*serviceSignature, error) {
	sig := &serviceSignature{
		APIVersion: server.svcName,
	}
	if server.service == nil {
		return sig, nil
	}
	methodMap := server.service.method
	mCount := len(methodMap)
	sig.Spec = make([]*refMethodSignature, mCount)
	m := 0
	var in []refFieldInfo
	var out []refFieldInfo
	var err error
	for name, method := range methodMap {
		argvType := indirectType(method.ArgType)
		replyType := method.ReplyType
		in, err = structToYaml(reflect.New(argvType).Elem().Addr().Interface())
		if err != nil {
			return nil, err
		}

		if replyType.Kind() == reflect.Interface {
			out = []refFieldInfo{}
		}
		if replyType.Kind() == reflect.Ptr {
			replyType = indirectType(replyType) //fix: 这里没有使用indirect导致下面的代码New了一个Interface
			out, err = structToYaml(reflect.New(replyType).Elem().Addr().Interface())
			if err != nil {
				return nil, err
			}
		}

		sig.Spec[m] = &refMethodSignature{
			Name: name,
			In:   in,
			Out:  out,
		}
		m++
	}
	return sig, nil
}

//输出服务函数的签名Yaml
func (server *daprServer) getSignatureYaml() (string, error) {
	sig := server.signature

	data, err := yaml.Marshal(sig)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

//在服务中增加一个函数签名校验方法
func (server *daprServer) invokeSignature(ctx context.Context, in *common.InvocationEvent) (*common.Content, error) {
	data, err := server.getSignatureYaml()
	if err != nil {
		return nil, err
	}

	return &common.Content{
		Data:        []byte(data),
		ContentType: "application/yaml",
	}, nil
}
