package dapr_sdk_warpper

import (
	"context"
	"errors"
	"fmt"
	"go/token"
	"log"
	"reflect"
	"strings"
	"sync"
)

var typeOfError = reflect.TypeOf((*error)(nil)).Elem()

type methodType struct {
	sync.Mutex // protects counters
	method     reflect.Method
	ArgType    reflect.Type
	ReplyType  reflect.Type
	numCalls   uint
}

type service struct {
	name   string                 // name of service
	rcvr   reflect.Value          // receiver of methods for the service
	typ    reflect.Type           // type of the receiver
	method map[string]*methodType // registered methods
}

// suitableMethods returns suitable Rpc methods of typ, it will report
// error using log if reportErr is true.
func suitableMethods(typ reflect.Type, reportErr bool) map[string]*methodType {
	methods := make(map[string]*methodType)
	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		mtype := method.Type
		mname := camelToLowerStyle(method.Name)
		// Method must be exported.
		if method.PkgPath != "" {
			continue
		}
		// Method needs three ins: receiver,context, *in, *out
		if mtype.NumIn() != 4 {
			if reportErr {
				log.Printf("rpc.Register: method %q has %d input parameters; needs exactly Four\n", mname, mtype.NumIn())
			}
			continue
		}
		//context

		fistType := mtype.In(1)
		ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
		if fistType != ctxType {
			if reportErr {
				log.Printf("rpc.Register: first argument must be a pointer method %q is not exported: %q\n", mname, fistType)
			}
			continue
		}

		// In arg must be a pointer.
		argType := mtype.In(2)
		if argType.Kind() != reflect.Ptr || !isExportedOrBuiltinType(argType) {
			if reportErr {
				log.Printf("rpc.Register: argument type of method %q is not exported: %q\n", mname, argType)
			}
			continue
		}
		// Out arg must be a pointer. 参数可以为interface{}或者指针
		replyType := mtype.In(3)
		if replyType.Kind() != reflect.Ptr && replyType.Kind() != reflect.Interface {
			if reportErr {
				log.Printf("rpc.Register: reply type of method %q is not a pointer: %q\n", mname, replyType)
			}
			continue
		}
		// Reply type must be exported.
		if !isExportedOrBuiltinType(replyType) {
			if reportErr {
				log.Printf("rpc.Register: reply type of method %q is not exported: %q\n", mname, replyType)
			}
			continue
		}
		// Method needs one out.
		if mtype.NumOut() != 1 {
			if reportErr {
				log.Printf("rpc.Register: method %q has %d output parameters; needs exactly one\n", mname, mtype.NumOut())
			}
			continue
		}
		// The return type of the method must be error.
		if returnType := mtype.Out(0); returnType != typeOfError {
			if reportErr {
				log.Printf("rpc.Register: return type of method %q is %q, must be error\n", mname, returnType)
			}
			continue
		}
		methods[mname] = &methodType{method: method, ArgType: argType, ReplyType: replyType}
	}
	return methods
}

// Is this type exported or a builtin?
func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// PkgPath will be non-empty even for an exported type,
	// so we need to check the type name as well.
	return token.IsExported(t.Name()) || t.PkgPath() == ""
}

func indirect(reflectValue reflect.Value) reflect.Value {
	for reflectValue.Kind() == reflect.Ptr {
		reflectValue = reflectValue.Elem()
	}
	return reflectValue
}

func indirectType(reflectType reflect.Type) (_ reflect.Type) {
	for reflectType.Kind() == reflect.Ptr || reflectType.Kind() == reflect.Slice {
		reflectType = reflectType.Elem()
	}
	return reflectType
}

func getJsonDataType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.String:
		return "string"
	default:
		return ""
	}

}

type refFieldInfo map[string]interface{}

func getStructFieldName(ft reflect.StructField) (string, bool) {
	tag := ft.Tag.Get("json")
	omitEmpty := false
	if strings.HasSuffix(tag, "omitempty") {
		omitEmpty = true
		idx := strings.Index(tag, ",")
		if idx > 0 {
			tag = tag[:idx]
		} else {
			tag = ft.Name
		}
	}
	return tag, omitEmpty
}

func structToYaml(inst interface{}) ([]refFieldInfo, error) {
	srcValue := indirect(reflect.ValueOf(inst))
	srcType := indirectType(reflect.TypeOf(inst))

	if srcType.Kind() != reflect.Struct {
		return nil, errors.New("inst must be a struct")
	}
	fieldInfoList := []refFieldInfo{}

	for m := 0; m < srcType.NumField(); m++ {
		field := srcType.Field(m)
		fieldName, _ := getStructFieldName(field)
		jsonTypeName := getJsonDataType(field.Type)
		isSimpleType := jsonTypeName != ""
		if field.Anonymous {
			ret, err := structToYaml(srcValue.Field(m).Addr().Interface())
			if err != nil {
				continue
			}
			fieldInfoList = append(fieldInfoList, ret...)
			continue
		}
		fieldInfo := make(refFieldInfo)
		//简单数据类型
		if isSimpleType {
			fieldInfo[fieldName] = jsonTypeName
			fieldInfoList = append(fieldInfoList, fieldInfo)
			continue
		}
		//2022-04-21 增加对时间类型支持
		if field.Type.Name() == "Time" {
			log.Panicf("not support time.Time use 'JSONTime'")
			continue
		}

		if field.Type.Name() == "JSONTime" {
			fieldInfo[fieldName] = "time"
			fieldInfoList = append(fieldInfoList, fieldInfo)
			continue
		}

		//Struct
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = indirectType(fieldType)
		}
		if fieldType.Kind() == reflect.Struct {
			ret, err := structToYaml(srcValue.Field(m).Addr().Interface())
			if err != nil {
				continue
			}
			fieldInfo[fieldName] = ret
			fieldInfoList = append(fieldInfoList, fieldInfo)
			continue
		}

		//数组
		if fieldType.Kind() == reflect.Array || fieldType.Kind() == reflect.Slice {
			fieldType = fieldType.Elem()
			if fieldType.Kind() == reflect.Ptr || fieldType.Kind() == reflect.Struct {
				fieldType = indirectType(fieldType) //fix: 引用错误，不能用Elem
				//递归对象检查
				fmt.Printf("%s %s\n", srcType.Name(), fieldType.Name())
				if srcType.Name() == fieldType.Name() {
					fieldInfo[fieldName] = []refFieldInfo{} //fix: 这里应当再加一层数组，用于表示field是个数组
					fieldInfoList = append(fieldInfoList, fieldInfo)
					continue
				}

				ptr := reflect.New(fieldType).Elem()
				ret, err := structToYaml(ptr.Addr().Interface())
				if err != nil {
					continue
				}
				fieldInfo[fieldName] = [][]refFieldInfo{ret} //fix: 这里应当再加一层数组，用于表示field是个数组
				fieldInfoList = append(fieldInfoList, fieldInfo)
				continue
			}
			json := getJsonDataType(fieldType)
			if json != "" {
				fieldInfo[fieldName] = []string{json}
				fieldInfoList = append(fieldInfoList, fieldInfo)
			}
			continue
		}

		if fieldType.Kind() == reflect.Map {
			fieldType = fieldType.Elem()
			if fieldType.Kind() != reflect.String {
				log.Fatalf("param not support lazy bind map, must be map[string]string")
			}
			fieldInfo[fieldName] = map[string]string{"string": "string"}
			fieldInfoList = append(fieldInfoList, fieldInfo)
			continue
		}

	}

	return fieldInfoList, nil

}
