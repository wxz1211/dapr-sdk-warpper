package dapr_sdk_warpper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"log"
	"os"
	"reflect"
	"time"

	"github.com/dapr/go-sdk/client"
	"github.com/dapr/go-sdk/service/common"
	dapr_grpc "github.com/dapr/go-sdk/service/grpc"
	dapr_http "github.com/dapr/go-sdk/service/http"
	validator "github.com/go-playground/validator/v10"
)

type ServerType uint
type JSONTime time.Time

const (
	GRPC ServerType = iota
	HTTP
)

type InvokeHandle func(ctx context.Context, in, out interface{}) error
type ServiceCreator func() common.Service

var defaultClient client.Client
var logger = log.New(os.Stderr, "hgmicro_sdk: ", log.LstdFlags)
var validate = validator.New()

type daprServer struct {
	svcName   string
	service   *service
	daprSvr   common.Service
	signature *serviceSignature
	svrType   ServerType
}

func newDaprServer() *daprServer {
	validate.SetTagName("binding")
	return &daprServer{}
}

//兼容gin-binding的参数校验
func validParam(v reflect.Value) error {
	if v.Kind() == reflect.Struct {
		return validate.Struct(v.Interface())
	} else if v.Kind() == reflect.Ptr {
		return validParam(v.Elem())
	}
	return nil
}

func (server *daprServer) register(rcvr interface{}, name string, useName bool) error {
	s := new(service)
	s.typ = reflect.TypeOf(rcvr)
	s.rcvr = reflect.ValueOf(rcvr)
	sname := reflect.Indirect(s.rcvr).Type().Name()
	if useName {
		sname = name
	}
	if sname == "" {
		s := "rpc.Register: no service name for type " + s.typ.String()
		logger.Print(s)
		return errors.New(s)
	}
	if !token.IsExported(sname) && !useName {
		s := "rpc.Register: type " + sname + " is not exported"
		logger.Print(s)
		return errors.New(s)
	}
	s.name = sname

	// Install the methods
	s.method = suitableMethods(s.typ, true)
	if len(s.method) == 0 {
		str := ""
		// To help the user, see if a pointer receiver would work.
		method := suitableMethods(reflect.PtrTo(s.typ), true)
		if len(method) != 0 {
			str = "rpc.Register: type " + sname + " has no exported methods of suitable type (hint: pass a pointer to value of that type)"
		} else {
			str = "rpc.Register: type " + sname + " has no exported methods of suitable type"
		}
		logger.Print(str)
		return errors.New(str)
	}
	server.svcName = sname
	server.service = s
	return nil
}

//RegistMethods 注册服务到RPC
//@Param className svr参数注册方法后，函数组的前缀
//@Param svr 函数组所在的Struct实例
func (server *daprServer) registMethods(className string, svr interface{}) error {
	if className == "" {
		return errors.New("className is empty")
	}

	err := server.register(svr, className, true)
	if err != nil {
		return fmt.Errorf("%s has not exported method", className)
	}

	sig, err := server.getSignature()
	if err != nil {
		return err
	}
	server.signature = sig
	return nil
}

func (server *daprServer) logMethodCall(name string, in *common.InvocationEvent, err interface{}) {
	var realErr error
	if err != nil {
		realErr = err.(error)
	}
	logger.Printf("exec [%s] by (%s) %s error:%v", name, string(in.Data), in.ContentType, realErr)
}

func (server *daprServer) invokeWarpper(mName string, receiver reflect.Value, mtype *methodType) common.ServiceInvocationHandler {
	return func(ctx context.Context, in *common.InvocationEvent) (*common.Content, error) {
		//1. 构造入参

		argv := reflect.New(mtype.ArgType.Elem())
		params := argv.Interface()
		if err := json.Unmarshal(in.Data, &params); err != nil {
			return nil, err
		}

		if err := validParam(argv); err != nil {
			return nil, err
		}
		//2. 构造出参
		var replyv reflect.Value
		if mtype.ReplyType.Kind() == reflect.Interface {
			//没有确定类型统一为nil
			replyv = reflect.ValueOf((*interface{})(nil))
		} else {
			//有确定类型，创建确定类型
			replyv = reflect.New(mtype.ReplyType.Elem())
			switch mtype.ReplyType.Elem().Kind() {
			case reflect.Map:
				replyv.Elem().Set(reflect.MakeMap(mtype.ReplyType.Elem()))
			case reflect.Slice:
				replyv.Elem().Set(reflect.MakeSlice(mtype.ReplyType.Elem(), 0, 0))
			}
		}

		//执行函数
		function := mtype.method.Func
		// Invoke the method, providing a new value for the reply.
		returnValues := function.Call([]reflect.Value{receiver, reflect.ValueOf(ctx), argv, replyv})
		// The return value for the method is an error.
		errInter := returnValues[0].Interface()
		server.logMethodCall(mName, in, errInter)
		if errInter != nil {
			return nil, errInter.(error)
		}
		if replyv.IsNil() {
			return nil, nil
		}
		data, err := json.Marshal(replyv.Interface())
		if err != nil {
			return nil, err
		}
		return &common.Content{
			Data:        data,
			ContentType: "application/json",
		}, nil

	}
}

func (server *daprServer) hook(daprd common.Service) error {
	if server.daprSvr != nil {
		return errors.New("dapr has already been hooked")
	}
	if server.service == nil {
		return errors.New("service has no method exported ")
	}
	logger.Printf("hook service %s to dapr", server.svcName)
	svcImpl := server.service
	for methodName, method := range svcImpl.method {
		logger.Printf("add method [%s] to invoke\n", methodName)

		err := daprd.AddServiceInvocationHandler(methodName, server.invokeWarpper(methodName, svcImpl.rcvr, method))
		if err != nil {
			return fmt.Errorf("add service [%s] error: %v", methodName, err)
		}
	}

	//外部可以通过此函数获取函数签名信息
	daprd.AddServiceInvocationHandler("get_signature", server.invokeSignature)
	server.daprSvr = daprd
	return nil
}

//设置自定义的Logger
func SetLogger(loggerImpl *log.Logger) {
	logger = loggerImpl
}

//NewServiceWithDapr 启动Dapr服务
//@address 监听的地址与端口号，格式如下：":2000" 等效于 "0.0.0.0:2000"
func NewServiceWithDapr(address string, svrType ServerType, className string, svr interface{}) (common.Service, error) {
	var svc common.Service
	var err error

	defaultDaprServer := newDaprServer()
	if err := defaultDaprServer.registMethods(className, svr); err != nil {
		return nil, err
	}
	data, err := defaultDaprServer.getSignatureYaml()
	if err == nil {
		logger.Printf("Service method signature\n%s\n", data)
	}

	defaultDaprServer.svrType = svrType
	if svrType == GRPC {
		svc, err = dapr_grpc.NewService(address)
		if err != nil {
			return nil, err
		}
	} else if svrType == HTTP {
		svc = dapr_http.NewService(address)
	}
	err = defaultDaprServer.hook(svc)
	if err != nil {
		logger.Panic(err.Error())
	}
	return svc, nil
}

//NewService 启动Dapr服务,外部手动创建不同类型的服务(grpc/http)
func NewService(service common.Service, className string, svr interface{}) error {

	var err error

	defaultDaprServer := newDaprServer()
	if err := defaultDaprServer.registMethods(className, svr); err != nil {
		return err
	}
	data, err := defaultDaprServer.getSignatureYaml()
	if err == nil {
		logger.Printf("Service method signature\n%s\n", data)
	}

	if service == nil {
		return errors.New("service is null")
	}
	err = defaultDaprServer.hook(service)
	if err != nil {
		logger.Panic(err.Error())
	}
	return nil
}

func GetClient() (client.Client, error) {
	if defaultClient != nil {
		return defaultClient, nil
	}
	return client.NewClient()
}

func Invoke(appId, method string, in interface{}, out interface{}) error {
	c, err := GetClient()
	if err != nil {
		return err
	}
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	resp, err := c.InvokeMethodWithContent(context.Background(), appId, method, "POST", &client.DataContent{
		Data:        data,
		ContentType: "application/json",
	})
	logger.Printf("invoke [%s.%s]\n in:`%s` out:`%s` error:%v", appId, method, string(data), string(resp), err)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err = json.Unmarshal(resp, out); err != nil {
		return err
	}
	return nil
}
